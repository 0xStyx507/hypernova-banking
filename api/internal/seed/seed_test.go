package seed

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateUniqueEmails(t *testing.T) {
	if err := validateUniqueEmails([]userRecord{{Email: "One@Example.com"}, {Email: "two@example.com"}}); err != nil {
		t.Fatalf("expected unique emails to pass: %v", err)
	}
	if err := validateUniqueEmails([]userRecord{{Email: "One@Example.com"}, {Email: "one@example.com"}}); err == nil {
		t.Fatal("expected duplicate emails to fail")
	}
}

func TestBuildDuplicateReportDoesNotExposeSecrets(t *testing.T) {
	report := buildDuplicateReport(fixture{
		Users: []userRecord{
			{ID: "user-one", Email: "same@example.com", Password: "password-one", FullName: "One", CreatedAt: "2026-01-01T00:00:00Z"},
			{ID: "user-two", Email: "SAME@example.com", Password: "password-two", FullName: "Two", CreatedAt: "2026-01-02T00:00:00Z"},
		},
		Accounts: []accountRecord{
			{AccountNumber: "account-one", UserID: "user-one"},
			{AccountNumber: "account-two", UserID: "user-two"},
		},
		Transactions: []transactionRecord{{FromAccount: "account-one", ToAccount: "account-two"}},
	})

	if report.DuplicateGroups != 1 || report.DuplicateRecords != 2 {
		t.Fatalf("unexpected duplicate totals: %+v", report)
	}
	conflict := report.Conflicts[0]
	if conflict.SameProfileFields || conflict.RecordsWithAccounts != 2 || conflict.RecordsWithTransactions != 2 {
		t.Fatalf("unexpected conflict summary: %+v", conflict)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("encode report: %v", err)
	}
	if strings.Contains(string(encoded), "same@example.com") || strings.Contains(string(encoded), "password-one") {
		t.Fatalf("report exposed fixture data: %s", encoded)
	}
}
