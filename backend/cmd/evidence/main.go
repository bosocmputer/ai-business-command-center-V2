// Command evidence keeps the proof behind a disputed number. When someone says a number was wrong, normal retention
// would delete the stored reports (after 90 days) and call logs (after 365) before the question is settled. A case copies
// the proof for one shop and a time window into one row that is kept three years and deleted with the shop.
//
//	/app/evidence capture <tenant-id> <from> <to> "<reason>" [your-name]   (dates YYYY-MM-DD, shop day; or RFC3339)
//	/app/evidence list <tenant-id>
//	/app/evidence show <tenant-id> <case-id>      (prints the whole copy as JSON: figures, maybe names; redirect it to a file)
//	/app/evidence delete <tenant-id> <case-id> [your-name]
//
// Everything except `show` prints counts and times only.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/config"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/database"
	"github.com/google/uuid"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "evidence:", err)
		os.Exit(2)
	}
}

var bangkok = time.FixedZone("Asia/Bangkok", 7*60*60)

func parseTime(text string, endOfDay bool) (time.Time, error) {
	text = strings.TrimSpace(text)
	if moment, err := time.Parse(time.RFC3339, text); err == nil {
		return moment.UTC(), nil
	}
	day, err := time.ParseInLocation(time.DateOnly, text, bangkok)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is neither YYYY-MM-DD nor RFC3339", text)
	}
	if endOfDay {
		return day.Add(24*time.Hour - time.Second).UTC(), nil
	}
	return day.UTC(), nil
}

func run(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: evidence capture|list|show|delete <tenant-id> ... (see the command's comment)")
	}
	tenantID, err := uuid.Parse(args[1])
	if err != nil {
		return errors.New("tenant id must be a UUID")
	}
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return errors.New("invalid configuration")
	}
	ctx := context.Background()
	pool, err := database.OpenPool(ctx, cfg.DatabaseURL, cfg.DatabaseMaxConnections, cfg.DatabaseMinConnections)
	if err != nil {
		return errors.New("database is unavailable")
	}
	defer pool.Close()
	store := database.NewEvidenceStore(pool)
	switch args[0] {
	case "capture":
		if len(args) < 5 {
			return errors.New("usage: evidence capture <tenant-id> <from> <to> \"<reason>\" [your-name]")
		}
		from, fromErr := parseTime(args[2], false)
		to, toErr := parseTime(args[3], true)
		if fromErr != nil || toErr != nil || to.Before(from) || to.Sub(from) > 93*24*time.Hour {
			return errors.New("the window must be ordered and at most 93 days, dates as YYYY-MM-DD")
		}
		reason := strings.TrimSpace(args[4])
		if reason == "" || len([]rune(reason)) > 300 {
			return errors.New("the reason must be 1 to 300 characters (say what was disputed, not who)")
		}
		who := "operator"
		if len(args) > 5 && strings.TrimSpace(args[5]) != "" {
			who = strings.TrimSpace(args[5])
		}
		summary, err := store.Capture(ctx, tenantID, from, to, reason, who, time.Now().UTC())
		if err != nil {
			return err
		}
		fmt.Printf("case %s kept until %s: %d calls, %d stored reports, %d cards, %d alerts, %d bytes\n", summary.ID, summary.ExpiresAt.In(bangkok).Format(time.DateOnly), summary.Calls, summary.Snapshots, summary.Cards, summary.Alerts, summary.Bytes)
	case "list":
		cases, err := store.List(ctx, tenantID)
		if err != nil {
			return err
		}
		for _, item := range cases {
			fmt.Printf("%s  made %s by %s  window %s..%s  %d calls, %d reports, %d cards, %d alerts  until %s  — %s\n", item.ID, item.CreatedAt.In(bangkok).Format("2006-01-02 15:04"), item.RequestedBy,
				item.WindowFrom.In(bangkok).Format(time.DateOnly), item.WindowTo.In(bangkok).Format(time.DateOnly), item.Calls, item.Snapshots, item.Cards, item.Alerts, item.ExpiresAt.In(bangkok).Format(time.DateOnly), item.Reason)
		}
		fmt.Printf("%d case(s)\n", len(cases))
	case "show":
		if len(args) < 3 {
			return errors.New("usage: evidence show <tenant-id> <case-id>")
		}
		caseID, err := uuid.Parse(args[2])
		if err != nil {
			return errors.New("case id must be a UUID")
		}
		payload, err := store.Payload(ctx, tenantID, caseID)
		if err != nil {
			return err
		}
		fmt.Println(payload)
	case "delete":
		if len(args) < 3 {
			return errors.New("usage: evidence delete <tenant-id> <case-id> [your-name]")
		}
		caseID, err := uuid.Parse(args[2])
		if err != nil {
			return errors.New("case id must be a UUID")
		}
		who := "operator"
		if len(args) > 3 && strings.TrimSpace(args[3]) != "" {
			who = strings.TrimSpace(args[3])
		}
		deleted, err := store.Delete(ctx, tenantID, caseID, who, time.Now().UTC())
		if err != nil {
			return err
		}
		fmt.Println("deleted:", deleted)
	default:
		return errors.New("unknown command " + args[0])
	}
	return nil
}
