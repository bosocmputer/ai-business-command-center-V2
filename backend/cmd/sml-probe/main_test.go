package main

import "testing"

func TestSplitStatementsAllowsOnlySingleSelects(t *testing.T) {
	got, err := splitStatements("select 1;\n---\nwith a as (select 2) select * from a\n---\n")
	if err != nil || len(got) != 2 || got[0] != "select 1" {
		t.Fatalf("statements = %q, %v", got, err)
	}
	for _, bad := range []string{
		"delete from ic_trans", "update ic_trans set total_amount = 0", "select 1; drop table ic_trans",
		"insert into x values (1)", "  ", "---\n---",
	} {
		if _, err := splitStatements(bad); err == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}
