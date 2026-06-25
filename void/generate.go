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
	"reconcile_status", "unmatch_reason", "oper_trigger", "auto_adj_txn_status", "auto_adj_acct_status",
	"reconcile_ref_id", "dlp_check_duplicate_key", "dpp_check_duplicate_key", "dcb_check_duplicate_key",
	"dlp_effective_date", "dlp_event_dtm", "dlp_from_loan_acct_no", "dlp_from_loan_acct_id",
	"dlp_txn_status", "dlp_txn_amt", "dlp_acct_status", "dlp_product_flow_type",
	"dpp_effective_date", "dpp_event_dtm", "dpp_from_main_loan_acct_no",
	"dpp_txn_status", "dpp_txn_amt", "dpp_cust_fee", "dpp_dcb_status",
	"dpp_dlp_status", "dpp_biller_id", "dpp_payment_ref1", "dpp_payment_ref2", "dpp_payment_ref3",
	"dpp_payment_ref4", "dpp_posting_type", "dcb_effective_date", "dcb_event_dtm", "dcb_from_loan_acct_id",
	"dcb_txn_status", "dcb_txn_amt", "dcb_cust_fee", "dcb_acct_sts", "dcb_posting_type",
}

var cases = []map[string]string{
	{"auto_adj_txn_status": `["DPP"]`, "dcb_txn_status": "SUCCESS", "dpp_txn_status": "DISB_VOIDED_TIMEOUT", "init_dcb_status": "VOID_TIMEOUT", "init_dlp_status": "N/A", "init_status": "FAILED"},
	{"auto_adj_txn_status": `["DPP"]`, "dcb_txn_status": "SUCCESS", "dpp_txn_status": "DISB_VOIDED_CLO_VOIDED_TIMEOUT", "init_dcb_status": "SUCCESS", "init_dlp_status": "VOID_TIMEOUT", "init_status": "FAILED"},
	{"auto_adj_txn_status": `["DPP"]`, "dcb_txn_status": "SUCCESS", "dpp_txn_status": "DISB_VOIDED_CLO_VOIDED_FAILED", "init_dcb_status": "SUCCESS", "init_dlp_status": "VOID_FAILED", "init_status": "FAILED"},
	{"auto_adj_txn_status": `["DPP"]`, "dcb_txn_status": "FAILED", "dpp_txn_status": "DISB_VOIDED_TIMEOUT", "init_dcb_status": "VOID_TIMEOUT", "init_dlp_status": "N/A", "init_status": "FAILED"},
	{"auto_adj_txn_status": `["DLP"]`, "dcb_txn_status": "FAILED", "dpp_txn_status": "DISB_VOIDED_TIMEOUT", "init_dcb_status": "VOID_TIMEOUT", "init_dlp_status": "N/A", "init_status": "FAILED"},
	{"auto_adj_txn_status": `["DPP"]`, "dcb_txn_status": "SUCCESS", "dpp_txn_status": "DISB_VOIDED_TIMEOUT", "init_dcb_status": "SUCCESS", "init_dlp_status": "N/A", "init_status": "COMPLETED"},
}

// sqlFormat converts interface{} values to pre-formatted SQL strings.
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
	csvFilename := fmt.Sprintf("VOID_ITMX_BILLERS_RECONCILE_UNMATCHED_%s.csv", csvDate.Format("20060102"))

	var csvRows [][]string
	sqlRowsMap := make(map[string]map[string]string)
	var sharedRefs []string

	for t := 0; t < times; t++ {
		caseItem := cases[t%len(cases)]
		sharedRef := newUUIDv7()

		csvRowMap := map[string]string{
			"reconcile_status":        "DLP Unmatch",
			"unmatch_reason":          "No Record DLP",
			"oper_trigger":            "Y",
			"auto_adj_txn_status":     caseItem["auto_adj_txn_status"],
			"auto_adj_acct_status":    `["DLP","DCB"]`,
			"reconcile_ref_id":        sharedRef,
			"dlp_check_duplicate_key":      "",
			"dpp_check_duplicate_key":      sharedRef,
			"dcb_check_duplicate_key":      sharedRef,
			"dlp_effective_date":           "",
			"dlp_event_dtm":               "",
			"dlp_from_loan_acct_no":       "",
			"dlp_from_loan_acct_id":       "",
			"dlp_txn_status":              "",
			"dlp_txn_amt":                 "",
			"dlp_acct_status":             "",
			"dlp_product_flow_type":       "",
			"dpp_effective_date":          now.Format("2006-01-02"),
			"dpp_event_dtm":               now.Format("2006-01-02 15:04"),
			"dpp_from_main_loan_acct_no":  "123-4-56789-012",
			"dpp_txn_status":              caseItem["dpp_txn_status"],
			"dpp_txn_amt":             "1000.00",
			"dpp_cust_fee":            "1000.00",
			"dpp_dcb_status":          "REVERTED",
			"dpp_dlp_status":          "REVERTED",
			"dpp_biller_id":           "10554803832937",
			"dpp_payment_ref1":        "1234",
			"dpp_payment_ref2":        "1234",
			"dpp_payment_ref3":        "1234",
			"dpp_payment_ref4":        "1234",
			"dpp_posting_type":        "OUTBOUND",
			"dcb_effective_date":      now.Format("2006-01-02"),
			"dcb_event_dtm":           now.Format("2006-01-02 15:04"),
			"dcb_from_loan_acct_id":   newUUIDv7(),
			"dcb_txn_status":          caseItem["dcb_txn_status"],
			"dcb_txn_amt":             "1000.00",
			"dcb_cust_fee":            "1000.00",
			"dcb_acct_sts":            "VOIDED",
			"dcb_posting_type":        "OUTBOUND",
		}
		var row []string
		for _, h := range csvHeader {
			row = append(row, csvRowMap[h])
		}
		csvRows = append(csvRows, row)

		// Build SQL row — store pre-formatted SQL fragments
		sqlValsRaw := map[string]interface{}{
			"ref_id":                    sharedRef,
			"payment_token":             "",
			"retrieval_ref_no":          fmt.Sprintf("%d", 100000000000+rand.Int63n(900000000000)),
			"cust_ref_id":               fmt.Sprintf("%d", 100000000000000+rand.Int63n(900000000000000)),
			"bpp_retrieval_ref_no":      nil,
			"bpp_txn_ref":               nil,
			"customer_note":             "Load Test",
			"from_acct_id":              newUUIDv7(),
			"status":                    caseItem["init_status"],
			"dcb_status":                caseItem["init_dcb_status"],
			"status_cd":                 "0000",
			"status_desc":               "Processing",
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
			"bill_payment_workflow":     "VOID",
			"to_acct_no":                "",
			"to_bank_cd":                "",
			"proc_cd":                   "",
			"terminal_type":             "",
			"category":                  "",
			"created_dtm":               now.Format("2006-01-02 15:04:05.000000"),
			"updated_dtm":               now.Format("2006-01-02 15:04:05.000000"),
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
			"transaction_type":          "PAYMENT",
			"transaction_date_time":     nil,
			"transaction_code":          "",
			"internal_account_id":       "",
			"transaction_class":         "D",
			"denomination":              "THB",
			"reversal_flag":             "",
			"tfr_dtm":                   nil,
			"fee_internal_account_id":   "",
			"fee_transaction_code":      "",
			"fee_transaction_amount":    "0.00",
			"fee_type":                  "",
			"posting_type":              "OUTBOUND",
			"effective_date":            nil,
			"dlp_status":                caseItem["init_dlp_status"],
			"state":                     "",
			"partner_ref_id":            nil,
			"from_main_account_no":      nil,
			"from_address":              nil,
			"to_main_account_no":        nil,
			"to_address":                nil,
			"is_force_success":          nil,
			"input_terminal":            "KEYIN",
			"from_bank_code":            "088",
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
			"settlement_date":           nil,
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

	os.WriteFile(filepath.Join(baseDir(), ".latest"), []byte(timestamp), 0644)

	fmt.Printf("Generated %d record(s)\n", len(csvRows))
	fmt.Printf("Work dir : %s\n", workDir)
	fmt.Printf("Raw CSV  : %s\n", rawCsvPath)
	fmt.Printf("\nEdit raw.csv if needed, then run:\n  go run . finalize\n")
}

func newUUIDv7() string {
	u, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return u.String()
}
