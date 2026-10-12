package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/line"
)

type fakeMenus struct {
	menus     []line.RichMenuInfo
	current   string
	uploadErr error
	deleted   []string
	created   int
	defaultTo string
	cleared   bool
}

func (f *fakeMenus) List(context.Context) ([]line.RichMenuInfo, error) { return f.menus, nil }
func (f *fakeMenus) DefaultID(context.Context) (string, error)         { return f.current, nil }
func (f *fakeMenus) Create(context.Context, line.RichMenu) (string, error) {
	f.created++
	return "new-menu", nil
}
func (f *fakeMenus) UploadImage(context.Context, string, string, []byte) error { return f.uploadErr }
func (f *fakeMenus) SetDefault(_ context.Context, id string) error             { f.defaultTo = id; return nil }
func (f *fakeMenus) ClearDefault(context.Context) error                        { f.cleared = true; return nil }
func (f *fakeMenus) Delete(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func pngOf(width, height int) []byte {
	var buffer bytes.Buffer
	_ = png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, width, height)))
	return buffer.Bytes()
}

func TestApplyMakesTheNewMenuTheDefaultAndDeletesOnlyEarlierMenusOfOurs(t *testing.T) {
	fake := &fakeMenus{menus: []line.RichMenuInfo{{ID: "old-ours", Name: line.RichMenuName}, {ID: "made-by-hand", Name: "console menu"}}}
	var out bytes.Buffer
	if err := apply(context.Background(), fake, line.DefaultMenuButtons(), pngOf(2500, 843), &out); err != nil {
		t.Fatal(err)
	}
	if fake.defaultTo != "new-menu" || len(fake.deleted) != 1 || fake.deleted[0] != "old-ours" {
		t.Fatalf("default=%q deleted=%v", fake.defaultTo, fake.deleted)
	}
}

func TestApplyRefusesAWrongPictureBeforeTouchingLINE(t *testing.T) {
	for name, picture := range map[string][]byte{"wrong size": pngOf(1200, 800), "not an image": []byte("hello"), "empty": nil} {
		fake := &fakeMenus{}
		if err := apply(context.Background(), fake, line.DefaultMenuButtons(), picture, &bytes.Buffer{}); err == nil || fake.created != 0 {
			t.Fatalf("%s: err=%v created=%d", name, err, fake.created)
		}
	}
}

func TestApplyDeletesTheNewMenuWhenThePictureIsRefused(t *testing.T) {
	fake := &fakeMenus{uploadErr: errors.New("LINE refused")}
	if err := apply(context.Background(), fake, line.DefaultMenuButtons(), pngOf(2500, 843), &bytes.Buffer{}); err == nil {
		t.Fatal("expected the refusal to be returned")
	}
	if len(fake.deleted) != 1 || fake.deleted[0] != "new-menu" || fake.defaultTo != "" {
		t.Fatalf("deleted=%v default=%q", fake.deleted, fake.defaultTo)
	}
}

func TestRemoveClearsTheDefaultOnlyWhenItIsOursAndKeepsOtherMenus(t *testing.T) {
	fake := &fakeMenus{menus: []line.RichMenuInfo{{ID: "ours", Name: line.RichMenuName}, {ID: "hand", Name: "console menu"}}, current: "hand"}
	var out bytes.Buffer
	if err := remove(context.Background(), fake, &out); err != nil {
		t.Fatal(err)
	}
	if fake.cleared || len(fake.deleted) != 1 || fake.deleted[0] != "ours" || strings.Contains(out.String(), "hand") {
		t.Fatalf("cleared=%v deleted=%v out=%q", fake.cleared, fake.deleted, out.String())
	}
}
