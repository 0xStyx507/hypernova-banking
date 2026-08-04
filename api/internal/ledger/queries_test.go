package ledger

import (
	"testing"
	"time"
)

func TestFilterHistoryAppliesFinancialFilters(t *testing.T) {
	created := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	items := []HistoryView{
		{TransferID: "deposit", Type: "deposit", Direction: "credit", Amount: "2500", CreatedAt: created},
		{TransferID: "transfer", Type: "transfer", Direction: "debit", Amount: "9000", CreatedAt: created.Add(time.Hour)},
	}
	filtered := FilterHistory(items, TransactionQuery{Type: "transfer", Direction: "debit", MinAmount: 5000})
	if len(filtered) != 1 || filtered[0].TransferID != "transfer" {
		t.Fatalf("unexpected filtered history: %+v", filtered)
	}
}

func TestSummarizeHistoryUsesMinorUnitStrings(t *testing.T) {
	items := []HistoryView{
		{Direction: "credit", Amount: "1250"},
		{Direction: "debit", Amount: "300"},
		{Direction: "debit", Amount: "invalid"},
	}
	summary := SummarizeHistory(items, "USD")
	if summary.Credits != "1250" || summary.Debits != "300" || summary.Net != "950" || summary.TransactionCount != 2 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}
