// Command provision is what the server script (deploy/provision.sh) asks, once a minute, what to start or stop for the assistants that were
// created from the admin page, and where it reports how that went. It runs inside the worker container, so it reaches the database but opens
// no network port.
//
//	/app/provision list                       one line per thing to do: <instance> <tenant-id> start|stop
//	/app/provision token <instance>           prints the assistant token waiting for that instance (nothing else, no newline); exit 1 when none
//	/app/provision report <instance> <CREATING|RUNNING|STOPPED|ERROR> [CODE]
//	/app/provision ack <instance>             removes the sealed token once the script has written it
//	/app/provision adopt <instance> <tenant-id>   records an assistant set up by hand as running (so the admin page shows it)
//
// Only `token` prints a secret, and only to the script that pipes it into a file.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/config"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/database"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/provision"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/secret"
	"github.com/google/uuid"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "provision:", err)
		os.Exit(2)
	}
}

func run(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: provision list|token|report|ack|adopt ... (see the command's comment)")
	}
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return errors.New("invalid configuration")
	}
	box, err := secret.NewBox(cfg.EncryptionMasterKey, cfg.EncryptionKeyID, rand.Reader)
	if err != nil {
		return errors.New("secret box is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.OpenPool(ctx, cfg.DatabaseURL, 2, 1)
	if err != nil {
		return errors.New("database is unavailable")
	}
	defer pool.Close()
	// Creating and stopping happen in the admin page, so no token issuer or settings are needed here.
	service := provision.NewService(database.NewProvisionStore(pool), box, nil, nil, time.Now)
	switch args[0] {
	case "list":
		work, err := service.Pending(ctx)
		if err != nil {
			return errors.New("cannot read the pending work")
		}
		for _, item := range work {
			fmt.Printf("%s %s %s\n", item.Name, item.TenantID, item.Action)
		}
		return nil
	case "token":
		if len(args) != 2 {
			return errors.New("usage: provision token <instance>")
		}
		token, err := service.Token(ctx, args[1])
		if err != nil {
			os.Exit(1)
		}
		fmt.Print(token)
		return nil
	case "report":
		if len(args) < 3 || len(args) > 4 {
			return errors.New("usage: provision report <instance> <status> [CODE]")
		}
		code := ""
		if len(args) == 4 {
			code = args[3]
		}
		if err := service.Report(ctx, args[1], provision.Status(args[2]), code); err != nil {
			return fmt.Errorf("not reported: %w", err)
		}
		return nil
	case "ack":
		if len(args) != 2 {
			return errors.New("usage: provision ack <instance>")
		}
		return service.Ack(ctx, args[1])
	case "adopt":
		if len(args) != 3 {
			return errors.New("usage: provision adopt <instance> <tenant-id>")
		}
		tenantID, err := uuid.Parse(args[2])
		if err != nil {
			return errors.New("tenant id must be a UUID")
		}
		return service.Adopt(ctx, args[1], tenantID)
	default:
		return errors.New("unknown command")
	}
}
