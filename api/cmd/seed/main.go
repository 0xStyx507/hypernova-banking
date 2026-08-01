// Command seed imports development identity data into PostgreSQL.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/hypernova-banking/api/internal/db"
	"github.com/hypernova-banking/api/internal/seed"
)

func main() {
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "PostgreSQL connection URL")
	fixturePath := flag.String("file", "../datos-prueba-HNL.json", "development fixture path")
	bcryptCost := flag.Int("bcrypt-cost", bcrypt.DefaultCost, "bcrypt cost for new seed users")
	duplicateReportPath := flag.String("duplicates-report", "", "optional safe JSON report path for duplicate emails")
	reportOnly := flag.Bool("report-only", false, "write the duplicate report and exit without connecting to PostgreSQL")
	flag.Parse()

	if *duplicateReportPath != "" {
		report, err := seed.DuplicateReportFromFile(*fixturePath)
		if err != nil {
			slog.Error("duplicate report failed", "error", err)
			os.Exit(1)
		}
		if err := seed.WriteDuplicateReport(*duplicateReportPath, report); err != nil {
			slog.Error("duplicate report write failed", "error", err)
			os.Exit(1)
		}
		fmt.Printf("duplicate report written: groups=%d records=%d path=%s\n", report.DuplicateGroups, report.DuplicateRecords, *duplicateReportPath)
		if *reportOnly {
			return
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := db.Open(ctx, *databaseURL)
	if err != nil {
		slog.Error("seed database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		slog.Error("seed migration failed", "error", err)
		os.Exit(1)
	}
	report, err := seed.Run(ctx, pool, *fixturePath, *bcryptCost)
	if err != nil {
		slog.Error("seed failed", "error", err)
		os.Exit(1)
	}
	fmt.Printf("seed complete: users=%d accounts_deferred=%d transactions_deferred=%d\n", report.UsersProcessed, report.AccountsDeferred, report.TransactionsDeferred)
}
