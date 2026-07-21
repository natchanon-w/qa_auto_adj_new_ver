package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// csvHeader mirrors TransferOnusAdjustmentDetail in
// payment-bulk-adj-transfer-onus-processor/app/batch/detail_parser.go —
// confirmed against a real production control-JSON download.
var csvHeader = []string{
	"reconcile_status", "unmatch_reason", "auto_adjust_status", "transaction_reference_id",
	"vfs_check_duplicate_key", "dcb_check_duplicate_key",
	"vfs_effective_date", "vfs_transaction_date", "vfs_transaction_status",
	"vfs_from_account_no", "vfs_from_pocket_no", "vfs_from_tm_account_id",
	"vfs_to_account_no", "vfs_to_pocket_no", "vfs_to_tm_account_id",
	"vfs_transaction_type", "vfs_posting_type",
	"vfs_transaction_amount", "vfs_transaction_fee", "vfs_channel",
	"dcb_effective_date", "dcb_transaction_date", "dcb_transaction_status",
	"dcb_from_tm_account_id", "dcb_to_tm_account_id",
	"dcb_transaction_amount", "dcb_transaction_fee",
}

var autoAdjustStatuses = []string{"COMPLETED", "FAILED"}

func sqlFormat(val interface{}) string {
	if val == nil {
		return "NULL"
	}
	switch v := val.(type) {
	case string:
		if v == "NULL" || v == "" {
			return "NULL"
		}
		return fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''"))
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%.2f", v)
	}
	return fmt.Sprintf("'%v'", val)
}

func newUUIDv7() string {
	u, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return u.String()
}

func cmdGenerate(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: go run . generate <records>")
		os.Exit(1)
	}
	times, err := strconv.Atoi(args[0])
	if err != nil || times < 1 {
		fmt.Printf("Invalid records value: %q — must be a positive integer\n", args[0])
		os.Exit(1)
	}

	now := time.Now()
	timestamp := now.Format("20060102_150405")
	workDir := filepath.Join(baseDir(), "work", timestamp)
	os.MkdirAll(workDir, os.ModePerm)

	cfg := readConfig()
	pathParts := strings.Split(strings.Trim(cfg.BasePath, "/"), "/")
	csvDateStr := pathParts[len(pathParts)-1]
	csvDate, err := time.Parse("2006-01-02", csvDateStr)
	if err != nil {
		fmt.Printf("Cannot parse date from base_path %q: %v\n", cfg.BasePath, err)
		os.Exit(1)
	}
	csvFilename := fmt.Sprintf("TRANSFER_ON_US_RECONCILE_AUTO_ADJUST_%s.csv", csvDate.Format("20060102"))

	var csvRows [][]string
	sqlRowsMap := make(map[string]map[string]string)
	var sharedRefs []string

	for t := 0; t < times; t++ {
		sharedRef := newUUIDv7()

		vfsAmount := fmt.Sprintf("%d", 100+rand.Intn(4901))
		dcbAmount := vfsAmount

		csvRowMap := map[string]string{
			"reconcile_status":         "VFS unmatch",
			"unmatch_reason":           "Unmatch Status Auto adjustment",
			"auto_adjust_status":       autoAdjustStatuses[t%len(autoAdjustStatuses)],
			"transaction_reference_id": sharedRef,
			"vfs_check_duplicate_key":  fmt.Sprintf("D%s%06d", now.Format("060102150405"), t),
			"dcb_check_duplicate_key":  fmt.Sprintf("DigitalPaymentProcessorClientID D%s%06d", now.Format("060102150405"), t),
			"vfs_effective_date":       now.Format("2006-01-02"),
			"vfs_transaction_date":     now.Format("2006-01-02 15:04:05"),
			"vfs_transaction_status":   "PROCESSING",
			"vfs_from_account_no":      fmt.Sprintf("%d", 1000000000+rand.Int63n(9000000000)),
			"vfs_from_pocket_no":       fmt.Sprintf("%d", 100000000000000+rand.Int63n(900000000000000)),
			"vfs_from_tm_account_id":   newUUIDv7(),
			"vfs_to_account_no":        fmt.Sprintf("%d", 1000000000+rand.Int63n(9000000000)),
			"vfs_to_pocket_no":         fmt.Sprintf("%d", 100000000000000+rand.Int63n(900000000000000)),
			"vfs_to_tm_account_id":     newUUIDv7(),
			"vfs_transaction_type":     "POCKET_TRANSFER",
			"vfs_posting_type":         "OUTBOUND_INBOUND",
			"vfs_transaction_amount":   vfsAmount,
			"vfs_transaction_fee":      "0",
			"vfs_channel":              "CLICX App",
			"dcb_effective_date":       now.Format("2006-01-02"),
			"dcb_transaction_date":     now.Format("2006-01-02 15:04:05"),
			"dcb_transaction_status":   "SUCCESS",
			"dcb_from_tm_account_id":   newUUIDv7(),
			"dcb_to_tm_account_id":     newUUIDv7(),
			"dcb_transaction_amount":   dcbAmount,
			"dcb_transaction_fee":      "0",
		}
		var row []string
		for _, h := range csvHeader {
			row = append(row, csvRowMap[h])
		}
		csvRows = append(csvRows, row)

		// internal_transfer_transactions — both legs are same-bank (KTB), unlike
		// transfer-offus which crosses banks.
		sqlValsRaw := map[string]interface{}{
			"req_channel":               "VB",
			"requester":                 "VB",
			"ref_id":                    sharedRef,
			"req_dtm":                   now.Format("2006-01-02 15:04:05"),
			"retrieval_ref_no":          fmt.Sprintf("%s%06d", now.Format("060102150405"), t),
			"pib_id":                    newUUIDv7(),
			"created_request_id":        newUUIDv7(),
			"amount":                    fmt.Sprintf("%s.00", vfsAmount),
			"payment_txn_ref":           fmt.Sprintf("D%s%06d", now.Format("060102150405"), t),
			"customer_note":             "Load Test",
			"from_acct_no":              fmt.Sprintf("%010d", 1000000000+rand.Int63n(9000000000)),
			"from_acct_id":              newUUIDv7(),
			"from_trans_code":           "MSTONUS",
			"from_internal_acct_id":     "SETTLEMENT_INTERNAL_TRANSFER",
			"from_acct_status":          "0",
			"from_acct_class":           "D",
			"from_acct_group":           "SAVINGS",
			"from_acct_type":            "ACCOUNT",
			"to_acct_no":                fmt.Sprintf("%010d", 1000000000+rand.Int63n(9000000000)),
			"to_acct_id":                newUUIDv7(),
			"to_trans_code":             "MSTINUS",
			"to_internal_acct_id":       "SETTLEMENT_INTERNAL_TRANSFER",
			"to_acct_status":            "0",
			"to_acct_class":             "D",
			"to_acct_group":             "SAVINGS",
			"to_acct_type":              "ACCOUNT",
			"status":                    "PROCESSING",
			"status_code":               "0000",
			"status_desc":               "Success",
			"payment_fee":               "0.00",
			"service_type":              "InternalTransfer",
			"created_dtm":               now.Format("2006-01-02 15:04:05.000"),
			"updated_dtm":               now.Format("2006-01-02 15:04:05.000"),
			"from_account_display_name": "",
			"from_account_name_th":      "สมเจตน์ ไตรพัฒนาพร",
			"from_account_name_en":      "Somjet Tripattanaporn",
			"to_account_display_name":   "",
			"to_account_name_th":        "สมเจตน์ ไตรพัฒนาพร",
			"to_account_name_en":        "Somjet Tripattanaporn",
			"denomination":              "THB",
			"tfr_dtm":                   now.Format("2006-01-02 15:04:05.000"),
			"transaction_type":          "FUND_TRANSFER",
			"resp_status_code":          "",
			"resp_status_desc":          "",
			"posting_type":              "OUTBOUND_INBOUND",
			"effective_date":            now.Format("2006-01-02"),
			"from_bank_code":            "006",
			"to_bank_code":              "006",
			"from_product_group":        "SAV",
			"from_product_type":         "SA01",
			"to_product_group":          "SAV",
			"to_product_type":           "SA01",
			"from_branch_code":          "001",
			"from_core_bank":            "DCB",
			"to_branch_code":            "001",
			"to_core_bank":              "DCB",
			"to_proxy_type":             nil,
			"to_proxy_value":            nil,
			"customer_ref_id":           nil,
			"from_main_account_no":      nil,
			"to_main_account_no":        nil,
		}

		sqlVals := make(map[string]string)
		for _, col := range sqlColumns {
			sqlVals[col] = sqlFormat(sqlValsRaw[col])
		}
		sqlRowsMap[sharedRef] = sqlVals
		sharedRefs = append(sharedRefs, sharedRef)
	}

	rawCsvPath := filepath.Join(workDir, "raw.csv")
	f, err := os.Create(rawCsvPath)
	if err != nil {
		panic(err)
	}
	f.WriteString(strings.Join(csvHeader, "|") + "\n")
	for _, row := range csvRows {
		f.WriteString(strings.Join(row, "|") + "\n")
	}
	f.Close()

	state := StateFile{
		GeneratedAt: now.Format(time.RFC3339),
		Timestamp:   timestamp,
		CsvFilename: csvFilename,
		SqlRows:     sqlRowsMap,
		SharedRefs:  sharedRefs,
	}
	stateBytes, _ := json.MarshalIndent(state, "", "  ")
	os.WriteFile(filepath.Join(workDir, "state.json"), stateBytes, 0644)
	os.WriteFile(filepath.Join(baseDir(), ".latest"), []byte(filepath.Base(workDir)+"\n"), 0644)

	fmt.Printf("Generated %d record(s)\n", len(csvRows))
	fmt.Printf("Work dir : %s\n", workDir)
	fmt.Printf("Raw CSV  : %s\n", rawCsvPath)
	fmt.Printf("\nEdit raw.csv if needed, then run:\n  go run . finalize\n")
}
