package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestOpenAIWSTurnPricingZeroValue 钉死 WS turn 定价的初始化语义：
// BeforeTurn 触发前 pricingAt 必须保持零，由调用方显式提供 turn 起始时间作为 fallback。
func TestOpenAIWSTurnPricingZeroValue(t *testing.T) {
	var p openAIWSTurnPricing
	require.True(t, p.currentOr(time.Time{}).IsZero(),
		"BeforeTurn 冻结前必须保持零值，交由 RecordUsage 回退记录时刻")
}

func TestOpenAIWSTurnPricingCurrentOr(t *testing.T) {
	fallback := time.Date(2024, time.January, 2, 2, 0, 0, 0, time.UTC)

	t.Run("frozen time takes precedence", func(t *testing.T) {
		frozen := fallback.Add(time.Minute)
		var p openAIWSTurnPricing
		p.freeze(frozen)
		require.Equal(t, frozen, p.currentOr(fallback))
	})

	t.Run("zero value falls back to turn start", func(t *testing.T) {
		var p openAIWSTurnPricing
		require.Equal(t, fallback, p.currentOr(fallback))
	})
}

// TestOpenAIWSTurnPricingFreezePerTurn 钉死每个 turn 的 BeforeTurn 都会覆盖
// 上一个 turn 的定价时刻：长连接跨峰谷时后续 turn 不得沿用旧时刻。
func TestOpenAIWSTurnPricingFreezePerTurn(t *testing.T) {
	var p openAIWSTurnPricing
	turn1 := time.Now().Add(-time.Hour)
	turn2 := time.Now()

	p.freeze(turn1)
	require.Equal(t, turn1, p.currentOr(time.Time{}))

	p.freeze(turn2)
	require.Equal(t, turn2, p.currentOr(time.Time{}), "后续 turn 必须使用自己的定价时刻")
}
