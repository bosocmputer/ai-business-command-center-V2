package executionmode

import (
	"strconv"
	"time"
)

// Thailand has no daylight saving, so a fixed offset is exact and needs no tzdata.
func bangkok() *time.Location { return time.FixedZone("Asia/Bangkok", 7*60*60) }

func itoa(value int) string { return strconv.Itoa(value) }
