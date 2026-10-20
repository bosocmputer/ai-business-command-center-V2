// Command erase removes a shop, or one person's LINE ID and activity, from AI-BCC on the operator's request. It runs inside the worker container
// through deploy/erase.sh. It prints counts of rows per table and never a name, a LINE ID or a figure.
//
//	erase shop <tenant-id> [--people]                                   shows what would go; changes nothing
//	erase shop <tenant-id> [--people] --execute --confirm <tenant-id>   deletes the shop (it must not be ACTIVE); --people also wipes the LINE ID
//	                                                                    of everyone who belonged to this shop only
//	erase person <recipient-id> [--allow-assistant-holder]              shows what would go; changes nothing
//	erase person <recipient-id> [--allow-assistant-holder] --execute --confirm <recipient-id>
//
// What it cannot reach is printed at the end: the shop assistant's own memory and files (assistant/forget.sh) and the database backups.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/config"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/database"
	"github.com/google/uuid"
)

const reminder = `ยังเหลือที่เครื่องมือนี้ไม่ได้ลบ:
 - ความจำและไฟล์ของเลขา AI ของร้าน (รัน assistant/forget.sh บนเซิร์ฟเวอร์)
 - ข้อมูลในชุดสำรองฐานข้อมูล (รายวันเก็บ 14 ชุด ลบเองเมื่อครบ ไม่ควรลบมือ)
 - บันทึกการตรวจสอบ (audit) ที่เก็บ 365 วัน`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "erase:", err)
		os.Exit(2)
	}
}

func run(args []string) error {
	if len(args) < 2 || (args[0] != "shop" && args[0] != "person") {
		return errors.New("usage: erase shop|person <id> [flags] (see the command's comment)")
	}
	kind := args[0]
	id, err := uuid.Parse(args[1])
	if err != nil {
		return errors.New("the id is not a valid uuid")
	}
	flags := flag.NewFlagSet(kind, flag.ContinueOnError)
	people := flags.Bool("people", false, "shop: also wipe the LINE ID of people who belonged to this shop only")
	holder := flags.Bool("allow-assistant-holder", false, "person: allow erasing the holder of a shop's assistant")
	execute := flags.Bool("execute", false, "really delete (without it only a plan is shown)")
	confirm := flags.String("confirm", "", "the id again, to confirm")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if *execute && *confirm != id.String() {
		return errors.New("--execute needs --confirm with the same id")
	}
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return errors.New("invalid configuration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := database.OpenPool(ctx, cfg.DatabaseURL, 2, 1)
	if err != nil {
		return errors.New("database is unavailable")
	}
	defer pool.Close()
	store := database.NewEraseStore(pool)
	if kind == "shop" {
		return shop(ctx, store, id, *people, *execute)
	}
	return person(ctx, store, id, *holder, *execute)
}

// keptTables are not deleted with a shop: the audit trail and the incident history stay (without the shop), by design.
var keptTables = map[string]bool{"audit_logs": true, "operational_incident_events": true, "operational_incident_subjects": true}

func printCounts(counts []database.TableCount) {
	for _, item := range database.SortedCounts(counts) {
		note := ""
		if keptTables[item.Table] {
			note = "   (ไม่ลบ: เก็บไว้ ตัดการผูกกับร้านออก)"
		}
		fmt.Printf("  %-34s %d%s\n", item.Table, item.Rows, note)
	}
}

func shop(ctx context.Context, store *database.EraseStore, id uuid.UUID, people, execute bool) error {
	if !execute {
		plan, err := store.PlanShop(ctx, id)
		if err != nil {
			return err
		}
		fmt.Printf("แผน (ยังไม่ลบอะไร): ร้านสถานะ %s\n", plan.Status)
		printCounts(plan.Tables)
		fmt.Printf("คนที่อยู่เฉพาะร้านนี้และจะยังมีรหัส LINE เหลือถ้าไม่ใส่ --people: %d\n", len(plan.People))
		if plan.Status == "ACTIVE" {
			fmt.Println("ร้านยังเปิดอยู่: ต้องปิดร้านในหน้า admin ก่อนจึงจะลบได้")
		}
		fmt.Println("ถ้าถูกต้อง รันซ้ำพร้อม --execute --confirm <id>")
		fmt.Println(reminder)
		return nil
	}
	result, err := store.EraseShop(ctx, id, people, time.Now().UTC())
	if err != nil {
		return err
	}
	fmt.Println("ลบร้านแล้ว แถวที่ลบ:")
	printCounts(result.Deleted)
	fmt.Printf("ลบรหัส LINE ของ %d คน\n", result.PeopleDone)
	fmt.Println(reminder)
	return nil
}

func person(ctx context.Context, store *database.EraseStore, id uuid.UUID, holder, execute bool) error {
	if !execute {
		plan, err := store.PlanPerson(ctx, id)
		if err != nil {
			return err
		}
		fmt.Printf("แผน (ยังไม่ลบอะไร): สถานะ %s\n", plan.Status)
		printCounts(plan.Tables)
		if plan.HoldsAssistants > 0 {
			fmt.Printf("คนนี้ถือเลขา AI ของร้าน %d ร้าน: ต้องใส่ --allow-assistant-holder และเลขาจะไม่มีผู้ถือ\n", plan.HoldsAssistants)
		}
		fmt.Println("จะลบชื่อและรหัส LINE ถอนสิทธิ์ทุกร้าน และลบสิ่งที่คนนี้ทำไว้ (เก็บแถวว่างไว้ให้ประวัติการส่งยังชี้ถูก)")
		fmt.Println("ถ้าถูกต้อง รันซ้ำพร้อม --execute --confirm <id>")
		fmt.Println(reminder)
		return nil
	}
	counts, err := store.ErasePerson(ctx, id, holder, time.Now().UTC())
	if err != nil {
		return err
	}
	fmt.Println("ลบข้อมูลของคนนี้แล้ว แถวที่ลบ:")
	printCounts(counts)
	fmt.Println(reminder)
	return nil
}
