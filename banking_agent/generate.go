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

var csvHeader = []string{
	"reconcile_status", "unmatch_reason", "auto_adjust_status", "transaction_reference_id",
	"vfs_check_duplicate_key", "dcb_check_duplicate_key", "bpp_check_duplicate_key",
	"vfs_effective_date", "vfs_transaction_date", "vfs_transaction_status",
	"vfs_from_account_no", "vfs_from_pocket_no", "vfs_from_tm_account_id",
	"vfs_transaction_amount", "vfs_transaction_fee", "vfs_banking_agent_fee",
	"vfs_company_code", "vfs_channel",
	"dcb_effective_date", "dcb_transaction_date", "dcb_transaction_status",
	"dcb_from_tm_account_id", "dcb_transaction_amount", "dcb_transaction_fee",
	"bpp_effective_date", "bpp_transaction_date", "bpp_transaction_status",
	"bpp_transaction_amount", "bpp_transaction_fee", "bpp_company_code",
	"bpp_reference_1", "bpp_reference_2", "bpp_reference_3", "bpp_reference_4",
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
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%.2f", v)
	}
	return fmt.Sprintf("'%v'", val)
}

func cmdGenerate(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: go run . generate <records> [type]")
		fmt.Println("  type: regular (default) | copay | all")
		os.Exit(1)
	}
	times, err := strconv.Atoi(args[0])
	if err != nil || times < 1 {
		fmt.Printf("Invalid records value: %q — must be a positive integer\n", args[0])
		os.Exit(1)
	}

	fileType := "regular"
	if len(args) > 1 {
		fileType = args[1]
	}
	if fileType != "regular" && fileType != "copay" && fileType != "all" {
		fmt.Printf("Invalid type: %q — must be 'regular', 'copay', or 'all'\n", fileType)
		os.Exit(1)
	}

	if fileType == "all" {
		regularDir := generateOne(times, "regular", false)
		copayDir := generateOne(times, "copay", false)
		fmt.Println("Generated both file types:")
		fmt.Printf("  regular : %s\n", regularDir)
		fmt.Printf("  copay   : %s\n", copayDir)
		fmt.Println()
		fmt.Println("Finalize + upload each work dir separately:")
		fmt.Printf("  go run . finalize %s\n", regularDir)
		fmt.Printf("  go run . upload   %s\n", regularDir)
		fmt.Printf("  go run . finalize %s\n", copayDir)
		fmt.Printf("  go run . upload   %s --keep\n", copayDir)
		return
	}

	generateOne(times, fileType, true)
}

func generateOne(times int, fileType string, printHint bool) string {
	now := time.Now()
	timestamp := fmt.Sprintf("%s_%s", now.Format("20060102_150405"), fileType)
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
	csvNamePrefix := "BANKING_AGENT_RECONCILE_AUTO_ADJUST"
	if fileType == "copay" {
		csvNamePrefix = "BANKING_AGENT_COPAY_RECONCILE_AUTO_ADJUST"
	}
	csvFilename := fmt.Sprintf("%s_%s.csv", csvNamePrefix, csvDate.Format("20060102"))

	var csvRows [][]string
	sqlRowsMap := make(map[string]map[string]string)
	var sharedRefs []string

	dtmFull := now.Format("2006-01-02 15:04:05.000000")

	for t := 0; t < times; t++ {
		sharedRef := newUUIDv7()

		vfsFromTMAcc := newUUIDv7()
		dcbFromTMAcc := newUUIDv7()

		csvRowMap := map[string]string{
			"reconcile_status":         "VFS unmatch",
			"unmatch_reason":           "Unmatch Status Auto adjustment",
			"auto_adjust_status":       autoAdjustStatuses[t%len(autoAdjustStatuses)],
			"transaction_reference_id": sharedRef,
			"vfs_check_duplicate_key":  fmt.Sprintf("D%s%06d", now.Format("060102150405"), t),
			"dcb_check_duplicate_key":  fmt.Sprintf("DigitalPaymentProcessorClientID D%s%06d", now.Format("060102150405"), t),
			"bpp_check_duplicate_key":  sharedRef,
			"vfs_effective_date":       now.Format("2006-01-02"),
			"vfs_transaction_date":     now.Format("2006-01-02 15:04:05"),
			"vfs_transaction_status":   "PROCESSING",
			"vfs_from_account_no":      fmt.Sprintf("%d", 1000000000+rand.Int63n(9000000000)),
			"vfs_from_pocket_no":       fmt.Sprintf("%d", 100000000000000+rand.Int63n(900000000000000)),
			"vfs_from_tm_account_id":   vfsFromTMAcc,
			"vfs_transaction_amount":   "1000",
			"vfs_transaction_fee":      "1000",
			"vfs_banking_agent_fee":    "1000",
			"vfs_company_code":         "1110",
			"vfs_channel":              "CLICX app",
			"dcb_effective_date":       now.Format("2006-01-02"),
			"dcb_transaction_date":     now.Format("2006-01-02 15:04:05"),
			"dcb_transaction_status":   "SUCCESS",
			"dcb_from_tm_account_id":   dcbFromTMAcc,
			"dcb_transaction_amount":   "1000",
			"dcb_transaction_fee":      "1000",
			"bpp_effective_date":       now.Format("2006-01-02"),
			"bpp_transaction_date":     now.Format("2006-01-02 15:04:05"),
			"bpp_transaction_status":   "SUCCESS",
			"bpp_transaction_amount":   "1000",
			"bpp_transaction_fee":      "1000",
			"bpp_company_code":         "1111",
			"bpp_reference_1":          fmt.Sprintf("%d", 800000000+rand.Int63n(100000000)),
			"bpp_reference_2":          "NULL",
			"bpp_reference_3":          "NULL",
			"bpp_reference_4":          "NULL",
		}

		var row []string
		for _, h := range csvHeader {
			row = append(row, csvRowMap[h])
		}
		csvRows = append(csvRows, row)

		sqlValsRaw := map[string]interface{}{
			"ref_id":                    sharedRef,
			"payment_token":             "",
			"retrieval_ref_no":          fmt.Sprintf("%d", 100000000000+rand.Int63n(900000000000)),
			"cust_ref_id":               fmt.Sprintf("%d", 100000000000000+rand.Int63n(900000000000000)),
			"bpp_retrieval_ref_no":      nil,
			"bpp_txn_ref":               nil,
			"customer_note":             "Load Test",
			"from_acct_id":              newUUIDv7(),
			"status":                    "PROCESSING",
			"dcb_status":                "",
			"status_cd":                 "DPP4016",
			"status_desc":               "Invalid account ownership",
			"biller_ref_type":           "payeeCode",
			"biller_ref_value":          "AEONHP",
			"biller_name_th":            "",
			"biller_name_en":            "",
			"to_display_name":           "",
			"channel_agent_id":          "Mobile",
			"customer_fee":              "0.00",
			"company_fee":               "0.00",
			"banking_agent_fee":         "0.00",
			"total_fee":                 "0.00",
			"payment_fee":               "0.00",
			"amount":                    "1.00",
			"ref1":                      "4090610197362600",
			"ref2":                      "",
			"ref3":                      "",
			"ref4":                      nil,
			"account_type":              "SAV",
			"source_of_fund":            "AC",
			"sof_type":                  `{"casa":{"sofAccount":"1234567890","sofBranchCode":"001","sofAccountName":"John Doe","currency":"THB","deductAmount":11,"toCurrencyCode":"","convertedAmount":0,"fromCostCenter":"","exchangeRate":0}}`,
			"req_by":                    "VB",
			"req_dtm":                   now.Format("2006-01-02 15:04:05"),
			"reverse_dtm":               nil,
			"bill_payment_workflow":     "BANKING_AGENT",
			"to_acct_no":                "",
			"to_bank_cd":                "",
			"proc_cd":                   "",
			"terminal_type":             "",
			"category":                  "",
			"created_dtm":               dtmFull,
			"updated_dtm":               dtmFull,
			"payment_txn_ref":           nil,
			"from_acct_no":              "0000000043",
			"pib_id":                    "",
			"dcb_created_request_id":    "",
			"print1":                    nil,
			"print2":                    nil,
			"print3":                    nil,
			"print4":                    nil,
			"print5":                    nil,
			"print6":                    nil,
			"print7":                    nil,
			"transaction_type":          "",
			"transaction_date_time":     nil,
			"transaction_code":          "",
			"internal_account_id":       "",
			"transaction_class":         "D",
			"denomination":              "",
			"reversal_flag":             "",
			"tfr_dtm":                   nil,
			"fee_internal_account_id":   "",
			"fee_transaction_code":      "",
			"fee_transaction_amount":    "0.00",
			"fee_type":                  "",
			"posting_type":              "",
			"effective_date":            nil,
			"dlp_status":                "",
			"state":                     "",
			"partner_ref_id":            nil,
			"from_main_account_no":      nil,
			"from_address":              nil,
			"to_main_account_no":        nil,
			"to_address":                nil,
			"is_force_success":          nil,
			"input_terminal":            nil,
			"from_bank_code":            nil,
			"from_account_display_name": nil,
			"from_account_name_th":      nil,
			"from_account_name_en":      nil,
			"from_province_code":        nil,
			"term_type":                 nil,
			"pan_id":                    nil,
			"terminal_id":               nil,
			"transferee_fee":            nil,
			"transferer_fee":            nil,
			"sender_fee":                nil,
			"instruction_id":            nil,
			"type_of_sender":            nil,
			"type_of_receiver":          nil,
			"share_flag":                nil,
			"mer_cat_code":              nil,
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
		FileType:    fileType,
		SqlRows:     sqlRowsMap,
		SharedRefs:  sharedRefs,
	}
	stateBytes, _ := json.MarshalIndent(state, "", "  ")
	os.WriteFile(filepath.Join(workDir, "state.json"), stateBytes, 0644)
	os.WriteFile(filepath.Join(baseDir(), ".latest"), []byte(timestamp), 0644)

	fmt.Printf("Generated %d record(s) [%s]\n", len(csvRows), fileType)
	fmt.Printf("Work dir : %s\n", workDir)
	fmt.Printf("Raw CSV  : %s\n", rawCsvPath)
	if printHint {
		fmt.Printf("\nEdit raw.csv if needed, then run:\n  go run . finalize\n")
	}
	fmt.Println()
	return workDir
}

func newUUIDv7() string {
	u, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return u.String()
}
