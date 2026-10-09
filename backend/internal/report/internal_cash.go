package report

import (
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// InternalCashFlags are cash book documents that move money between the shop's own accounts: deposit (401), withdrawal (402),
// their cancellations (403), petty cash return (302/303/301) and cancelled transfer in (423). The cash receipts and payments
// reports count every cash book row, as their definitions say, so these are inside the totals. They are not income or spending,
// so the reports also report how much of the total they are (the KPI internal_move_amount) and leave the total as it was.
var InternalCashFlags = map[int]bool{401: true, 402: true, 403: true, 301: true, 302: true, 303: true, 423: true}

// internalCashFlagList is the same set as SQL text for an IN list: "301, 302, ...".
func internalCashFlagList() string {
	codes := make([]int, 0, len(InternalCashFlags))
	for code := range InternalCashFlags {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	parts := make([]string, len(codes))
	for index, code := range codes {
		parts[index] = strconv.Itoa(code)
	}
	return strings.Join(parts, ", ")
}

// internalMoveAmount adds up the rows that are internal moves, from detail rows that carry their document code.
func internalMoveAmount(rows []map[string]string) (*big.Rat, error) {
	total := new(big.Rat)
	for _, row := range rows {
		code, err := strconv.Atoi(strings.TrimSpace(row["trans_flag_code"]))
		if err != nil || !InternalCashFlags[code] {
			continue
		}
		amount, err := decimal(row["total_amount"])
		if err != nil {
			return nil, fieldDecimalError("total_amount", err)
		}
		total.Add(total, amount)
	}
	return total, nil
}
