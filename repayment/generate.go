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
	"reconcile_status", "unmatch_reason", "oper_trigger", "auto_adj_transfer_status", "auto_adj_payment_status",
	"reconcile_ref_id", "dlp_channel_txn_ref_id", "dpp_ref_id", "dlp_effective_date", "dlp_event_dtm",
	"dlp_from_acct_no", "dlp_to_loan_acct_id", "dlp_to_loan_acct_no", "dlp_transfer_status", "dlp_payment_status",
	"dlp_txn_amt", "dlp_principal", "dlp_interest", "dlp_penalty_interest", "dlp_collection_fee",
	"dpp_effective_date", "dpp_event_dtm", "dpp_from_acct_no", "dpp_from_acct_id", "dpp_to_loan_acct_no",
	"dpp_to_loan_acct_id", "dpp_transfer_status", "dpp_txn_amt", "dpp_txn_fee", "dcb_effective_date",
	"dcb_event_dtm", "dcb_from_acct_id", "dcb_to_loan_acct_id", "dcb_transfer_status", "dcb_payment_status",
	"dcb_txn_amt", "dcb_principal", "dcb_interest", "dcb_penalty_interest", "dcb_collection_fee",
}

var cases = []map[string]string{
	{"auto_adj_transfer_status": "DPP", "dcb_transfer_status": "SUCCESS", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PRCS", "dcb_payment_status": "NULL", "dlp_payment_status": "PENDING", "after_status": "COMPLETED"},
	{"auto_adj_transfer_status": "DPP", "dcb_transfer_status": "SUCCESS", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PRCS", "dcb_payment_status": "SUCCESS", "dlp_payment_status": "COMPLETED", "after_status": "COMPLETED"},
	{"auto_adj_transfer_status": "DPP", "dcb_transfer_status": "SUCCESS", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PRCS", "dcb_payment_status": "SUCCESS", "dlp_payment_status": "PENDING", "after_status": "COMPLETED"},
	{"auto_adj_transfer_status": "DPP", "dcb_transfer_status": "SUCCESS", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PRCS", "dcb_payment_status": "FAILED", "dlp_payment_status": "FAILED", "after_status": "COMPLETED"},
	{"auto_adj_transfer_status": "DPP", "dcb_transfer_status": "SUCCESS", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PRCS", "dcb_payment_status": "FAILED", "dlp_payment_status": "PENDING", "after_status": "COMPLETED"},
	{"auto_adj_transfer_status": "DPP", "dcb_transfer_status": "FAILED", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PRCS", "dcb_payment_status": "NULL", "dlp_payment_status": "PENDING", "after_status": "FAILED"},
	{"auto_adj_transfer_status": "DLP", "dcb_transfer_status": "FAILED", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PRCS", "dcb_payment_status": "NULL", "dlp_payment_status": "COMPLETED", "after_status": "No Update"},
	{"auto_adj_transfer_status": "DPP", "dcb_transfer_status": "FAILED", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PRCS", "dcb_payment_status": "SUCCESS", "dlp_payment_status": "PENDING", "after_status": "No Update"},
}

// sqlFormat converts interface{} values to SQL-safe strings stored in state.json.
// finalize.go emits these verbatim (no additional quoting).
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
	csvFilename := fmt.Sprintf("REPAYMENT_MANUAL_DDR_RECONCILE_UNMATCHED_%s.csv", csvDate.Format("20060102"))

	var csvRows [][]string
	sqlRowsMap := make(map[string]map[string]string)
	var sharedRefs []string

	dtmFull := now.Format("2006-01-02 15:04:05.000000 +00:00")

	for t := 0; t < times; t++ {
		caseItem := cases[t%len(cases)]
		sharedRef := newUUIDv7()

		csvRowMap := map[string]string{
			"reconcile_status":         "DLP Unmatch",
			"unmatch_reason":           "No Record DLP",
			"oper_trigger":             "Y",
			"auto_adj_transfer_status": fmt.Sprintf(`["%s"]`, caseItem["auto_adj_transfer_status"]),
			"auto_adj_payment_status":  `["DPP","DCB"]`,
			"reconcile_ref_id":         sharedRef,
			"dlp_channel_txn_ref_id":   "NULL",
			"dpp_ref_id":               sharedRef,
			"dlp_effective_date":       "NULL",
			"dlp_event_dtm":            "NULL",
			"dlp_from_acct_no":         "NULL",
			"dlp_to_loan_acct_id":      "NULL",
			"dlp_to_loan_acct_no":      "NULL",
			"dlp_transfer_status":      caseItem["dlp_transfer_status"],
			"dlp_payment_status":       caseItem["dlp_payment_status"],
			"dlp_txn_amt":              "NULL",
			"dlp_principal":            "NULL",
			"dlp_interest":             "NULL",
			"dlp_penalty_interest":     "NULL",
			"dlp_collection_fee":       "NULL",
			"dpp_effective_date":       now.Format("2006-01-02"),
			"dpp_event_dtm":            now.Format("2006-01-02 15:04:05"),
			"dpp_from_acct_no":         fmt.Sprintf("%d", 100000000000+rand.Int63n(900000000000)),
			"dpp_from_acct_id":         newUUIDv7(),
			"dpp_to_loan_acct_no":      "1234567890",
			"dpp_to_loan_acct_id":      newUUIDv7(),
			"dpp_transfer_status":      caseItem["dpp_transfer_status"],
			"dpp_txn_amt":              "1850.00",
			"dpp_txn_fee":              "10.00",
			"dcb_effective_date":       now.Format("2006-01-02"),
			"dcb_event_dtm":            now.Format("2006-01-02 15:04:05"),
			"dcb_from_acct_id":         newUUIDv7(),
			"dcb_to_loan_acct_id":      newUUIDv7(),
			"dcb_transfer_status":      caseItem["dcb_transfer_status"],
			"dcb_payment_status":       caseItem["dcb_payment_status"],
			"dcb_txn_amt":              "1850.00",
			"dcb_principal":            "1500.00",
			"dcb_interest":             "200.00",
			"dcb_penalty_interest":     "100.00",
			"dcb_collection_fee":       "50.00",
		}
		var row []string
		for _, h := range csvHeader {
			row = append(row, csvRowMap[h])
		}
		csvRows = append(csvRows, row)

		// Build SQL row — store pre-formatted SQL fragments
		cifNo := fmt.Sprintf("%015d", rand.Int63n(999999999999999))
		dcbReq := fmt.Sprintf(`{"clientId": "DigitalPaymentProcessorClientID", "requestId": "%s", "effectiveDate": "%s", "transactionDatetime": "%sT%s+07:00"}`,
			sharedRef, now.Format("20060102"), now.Format("2006-01-02"), now.Format("15:04:05"))
		dcbResp := fmt.Sprintf(`{"code": "0000", "message": "Success", "data": {"createdRequestId": "%s", "createdDatetime": "%s"}}`,
			sharedRef, now.Format("2006-01-02T15:04:05.000+07:00"))
		sqlValsRaw := map[string]interface{}{
			"ref_id":                    sharedRef,
			"original_ref_id":           "",
			"req_channel":               "VB",
			"requester":                 "DLP",
			"req_dtm":                   dtmFull,
			"ref_no":                    fmt.Sprintf("%012d", rand.Int63n(999999999999)),
			"payment_txn_ref":           fmt.Sprintf("D07D2%s", now.Format("0102150405")),
			"tfr_dtm":                   dtmFull,
			"created_request_id":        sharedRef,
			"amount":                    "1850.00",
			"denomination":              "THB",
			"customer_note":             "PP Initial Repayment",
			"customer_ref_id":           cifNo,
			"from_main_account_id":      nil,
			"from_main_account_no":      "Usi+DSZqYbZl1lRRpL/nIjnRSQCRTQWAetunbPKb5ApjsQkcAQc=",
			"from_transfer_account_no":  "170000002110001",
			"from_acct_no":              "Z7ZTjKYAk5gIS//YfASOiZTvJndj8nuEUVNl2huNC1kWI4tVbSE=",
			"from_acct_id":              newUUIDv7(),
			"to_internal_acct_id":       "FUND_TRANSFER",
			"from_trans_code":           "MLRPMIN",
			"from_address":              nil,
			"from_acct_status":          "0",
			"from_product_class":        "D",
			"from_product_group":        "SAV",
			"from_product_type":         "SA01",
			"from_acct_type":            "POCKET",
			"from_branch_code":          "46",
			"from_account_display_name": "ujxJDu2G8UjTUveQgXAmRv6cgtzcKAC4nSsc/cWfi2e0Euf+PT8Pz8d7fE6eYYUsFBc=",
			"from_account_name_th":      "Xl55Jw6nVJkzRjW5t2btXiG7HqyzYizgXCY5Yq/WHWCx+BKYTfu4O3IOazkS1kDcrok=",
			"from_account_name_en":      "TC+gIPolPnryLYBIPHuKs1Q4onB2FjGOsO8bXSt1QjIBpg==",
			"from_bank_code":            "088",
			"from_core_bank_channel":    "DCB",
			"from_cif_no":               cifNo,
			"from_cdi_token":            nil,
			"from_internal_acct_id":     "INTERNAL",
			"to_main_account_id":        nil,
			"to_main_account_no":        nil,
			"to_transfer_account_no":    nil,
			"to_acct_no":                "qJ3v/Xz8Npd4WLxJKM8plHEYWlR7BFhH4BR4Gb6aWg33qXOtlFubjQ==",
			"to_acct_id":                newUUIDv7(),
			"to_trans_code":             "MLRPIN",
			"to_address":                nil,
			"to_acct_status":            nil,
			"to_product_class":          "L",
			"to_product_group":          "LENDING",
			"to_product_type":           "LOAN",
			"to_acct_type":              "LOAN",
			"to_branch_code":            nil,
			"to_account_display_name":   "",
			"to_account_name_th":        "",
			"to_account_name_en":        "",
			"to_bank_code":              "088",
			"to_core_bank_channel":      "DCB",
			"to_cif_no":                 cifNo,
			"to_cdi_token":              cifNo,
			"fee_amount":                0.00,
			"fee_internal_acct_id":      nil,
			"fee_trans_code":            nil,
			"fee_type":                  nil,
			"status":                    "PRCS",
			"status_code":               "0000",
			"status_desc":               "Processing",
			"debit_status":              nil,
			"credit_status":             nil,
			"service_type":              "Repayment",
			"created_dtm":               dtmFull,
			"updated_dtm":               dtmFull,
			"transaction_type":          "REPAYMENT_TDR_INITIATE",
			"posting_type":              "OUTBOUND_CUSTOM",
			"effective_date":            now.Format("2006-01-02"),
			"reversal_flag":             "N",
			"transfer_type":             "MANUAL_INITIAL",
			"input_terminal":            "KEYIN",
			"dcb_request":               dcbReq,
			"dcb_response":              dcbResp,
			"debit_ref_id":              nil,
			"credit_ref_id":             nil,
			"user_id":                   nil,
			"trans_location":            nil,
			"from_comment":              nil,
			"to_comment":                nil,
			"source_of_payment":         nil,
			"erp_info_debit":            nil,
			"erp_info_credit":           nil,
			"info":                      nil,
			"term_id":                   nil,
			"term_type":                 nil,
			"waive_flag":                nil,
			"verify_ref":                nil,
			"term_branch":               nil,
			"adjusted_amount":           nil,
			"override_amount":           nil,
			"is_pay_off":                false,
		}

		// Pre-format all values as SQL fragments
		sqlVals := make(map[string]string)
		for _, col := range sqlColumns {
			sqlVals[col] = sqlFormat(sqlValsRaw[col])
		}
		// these NOT NULL columns default to '' — must not be NULL
		sqlVals["original_ref_id"] = "''"
		sqlVals["to_account_display_name"] = "''"
		sqlVals["to_account_name_th"] = "''"
		sqlVals["to_account_name_en"] = "''"
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
