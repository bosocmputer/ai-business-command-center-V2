package report

import "testing"

func TestRecommendModeUsesEachReportsOwnLine(t *testing.T) {
	for _, test := range []struct {
		key   Key
		units int
		want  ExecutionMode
	}{
		{StockBalance, ChunkUnitThreshold(StockBalance) - 1, ModeDirect},
		{StockBalance, ChunkUnitThreshold(StockBalance), ModeChunked},
		{ARCustomerMovement, ChunkUnitThreshold(StockBalance), ModeDirect},
		{ARCustomerMovement, ChunkUnitThreshold(ARCustomerMovement) - 1, ModeDirect},
		{ARCustomerMovement, ChunkUnitThreshold(ARCustomerMovement), ModeChunked},
		{SalesGoodsServices, 1_000_000, ModeDirect},
	} {
		if got := RecommendMode(test.key, test.units); got != test.want {
			t.Errorf("RecommendMode(%s, %d) = %s, want %s", test.key, test.units, got, test.want)
		}
	}
}

func TestEveryChunkableReportHasALineAndNoOtherReportDoes(t *testing.T) {
	for _, definition := range Definitions() {
		threshold := ChunkUnitThreshold(definition.Key)
		if definition.ChunkSafe && threshold < MinimumChunkSize {
			t.Errorf("%s can be chunked but its line is %d", definition.Key, threshold)
		}
		if !definition.ChunkSafe && threshold != 0 {
			t.Errorf("%s cannot be chunked but has a line of %d", definition.Key, threshold)
		}
	}
}
