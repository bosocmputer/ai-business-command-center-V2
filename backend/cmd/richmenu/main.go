// Command richmenu sets, shows or removes the button menu at the bottom of the owner's chat on the shared LINE channel. It runs inside the
// worker container, which already holds the channel's access token; nothing is printed except menu ids and names.
//
//	/app/richmenu status                         the menus of the channel and which one every user gets
//	/app/richmenu apply < menu.png               creates the menu with that picture (PNG or JPEG, 2500x843, at most 1 MB), makes it the
//	                                             default for every user, then deletes the earlier menus this command made
//	/app/richmenu remove                         takes the default menu away and deletes the menus this command made
//
// Only menus named like ours are ever deleted, so a menu made by hand in the LINE console is left alone.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/config"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/line"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "richmenu:", err)
		os.Exit(2)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: richmenu status | apply < picture | remove")
	}
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return errors.New("invalid configuration")
	}
	client := line.NewRichMenuClient(cfg.LineMessagingAccessToken, line.DefaultRichMenuAPI, line.DefaultRichMenuData, 20*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	switch args[0] {
	case "status":
		return status(ctx, client, stdout)
	case "apply":
		image, err := io.ReadAll(io.LimitReader(stdin, line.MaxRichMenuImage+1))
		if err != nil {
			return errors.New("the picture could not be read")
		}
		return apply(ctx, client, line.DefaultMenuButtons(), image, stdout)
	case "remove":
		return remove(ctx, client, stdout)
	}
	return errors.New("usage: richmenu status | apply < picture | remove")
}

type menuClient interface {
	List(ctx context.Context) ([]line.RichMenuInfo, error)
	DefaultID(ctx context.Context) (string, error)
	Create(ctx context.Context, menu line.RichMenu) (string, error)
	UploadImage(ctx context.Context, id, contentType string, image []byte) error
	SetDefault(ctx context.Context, id string) error
	ClearDefault(ctx context.Context) error
	Delete(ctx context.Context, id string) error
}

func status(ctx context.Context, client menuClient, out io.Writer) error {
	menus, err := client.List(ctx)
	if err != nil {
		return err
	}
	current, err := client.DefaultID(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "menus: %d\n", len(menus))
	for _, menu := range menus {
		marker := ""
		if menu.ID == current {
			marker = "  <- default for every user"
		}
		fmt.Fprintf(out, "  %s  %q%s\n", menu.ID, menu.Name, marker)
	}
	if current == "" {
		fmt.Fprintln(out, "no default menu")
	}
	return nil
}

func apply(ctx context.Context, client menuClient, buttons []line.MenuButton, picture []byte, out io.Writer) error {
	contentType := line.ImageContentType(picture)
	if contentType == "" {
		return errors.New("the picture must be PNG or JPEG")
	}
	if len(picture) > line.MaxRichMenuImage {
		return errors.New("the picture is larger than 1 MB")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(picture))
	if err != nil || config.Width != line.RichMenuWidth || config.Height != line.RichMenuHeight {
		return fmt.Errorf("the picture must be %dx%d pixels", line.RichMenuWidth, line.RichMenuHeight)
	}
	menu, err := line.BuildRichMenu(buttons)
	if err != nil {
		return err
	}
	before, err := client.List(ctx)
	if err != nil {
		return err
	}
	id, err := client.Create(ctx, menu)
	if err != nil {
		return err
	}
	if err := client.UploadImage(ctx, id, contentType, picture); err != nil {
		_ = client.Delete(ctx, id) // a menu without a picture is useless: do not leave it behind
		return err
	}
	if err := client.SetDefault(ctx, id); err != nil {
		_ = client.Delete(ctx, id)
		return err
	}
	fmt.Fprintf(out, "default menu is now %s\n", id)
	for _, old := range before {
		if old.Name == line.RichMenuName && old.ID != id {
			if err := client.Delete(ctx, old.ID); err != nil {
				fmt.Fprintf(out, "could not delete the earlier menu %s: %v\n", old.ID, err)
				continue
			}
			fmt.Fprintf(out, "deleted the earlier menu %s\n", old.ID)
		}
	}
	return nil
}

func remove(ctx context.Context, client menuClient, out io.Writer) error {
	menus, err := client.List(ctx)
	if err != nil {
		return err
	}
	current, err := client.DefaultID(ctx)
	if err != nil {
		return err
	}
	ours := map[string]bool{}
	for _, menu := range menus {
		if menu.Name == line.RichMenuName {
			ours[menu.ID] = true
		}
	}
	if current != "" && ours[current] {
		if err := client.ClearDefault(ctx); err != nil {
			return err
		}
		fmt.Fprintln(out, "default menu removed")
	}
	for id := range ours {
		if err := client.Delete(ctx, id); err != nil {
			return err
		}
		fmt.Fprintf(out, "deleted %s\n", id)
	}
	if len(ours) == 0 {
		fmt.Fprintln(out, "no menu of ours to remove")
	}
	return nil
}
