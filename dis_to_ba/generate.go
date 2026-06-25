package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var csvHeader = []string{
	"reconcile_status", "unmatch_reason", "oper_trigger", "auto_adj_txn_status", "auto_adj_acct_status",
	"reconcile_ref_id", "dcb_check_duplicate_key", "dlp_check_duplicate_key", "dpp_check_duplicate_key",
	"bpp_check_duplicate_key", "dlp_effective_date", "dlp_event_dtm", "dlp_from_loan_acct_no",
	"dlp_from_loan_acct_id", "dlp_txn_status", "dlp_txn_amt", "dlp_acct_status", "dlp_product_flow_type",
	"dpp_effective_date", "dpp_event_dtm", "dpp_from_loan_acct_no", "dpp_txn_status", "dpp_txn_amt",
	"dpp_cust_fee", "dpp_banking_agent_fee", "dpp_dcb_status", "dpp_dlp_status", "dpp_comp_code",
	"dpp_posting_type", "dcb_effective_date", "dcb_event_dtm", "dcb_from_loan_acct_id", "dcb_txn_status",
	"dcb_txn_amt", "dcb_cust_fee", "dcb_acct_status", "dcb_posting_type", "bpp_effective_date",
	"bpp_event_dtm", "bpp_txn_status", "bpp_txn_amt", "bpp_banking_agent_fee", "bpp_comp_code",
}

var cases = []map[string]map[string]string{
	{"csv": {"auto_adj_txn_status": `["DPP"]`, "bpp_txn_status": "SUCCESS", "dcb_txn_status": "SUCCESS", "dpp_txn_status": "PRCS"}, "sql": {"status": "PROCESSING", "dcb_status": "", "dlp_status": ""}},
	{"csv": {"auto_adj_txn_status": `["DPP"]`, "bpp_txn_status": "FAILED", "dcb_txn_status": "REVERSED", "dpp_txn_status": "DISB_REV_CLO_REV_FAILED"}, "sql": {"status": "FAILED", "dcb_status": "", "dlp_status": "REVERSED_FAILED"}},
	{"csv": {"auto_adj_txn_status": `["DPP"]`, "bpp_txn_status": "FAILED", "dcb_txn_status": "REVERSED", "dpp_txn_status": "DISB_REV_CLO_REV_TIMEOUT"}, "sql": {"status": "FAILED", "dcb_status": "", "dlp_status": "REVERSED_TIMEOUT"}},
	{"csv": {"auto_adj_txn_status": `["DPP"]`, "bpp_txn_status": "", "dcb_txn_status": "FAILED", "dpp_txn_status": "DISB_REV_FAILED"}, "sql": {"status": "FAILED", "dcb_status": "", "dlp_status": ""}},
	{"csv": {"auto_adj_txn_status": `["DPP"]`, "bpp_txn_status": "", "dcb_txn_status": "FAILED", "dpp_txn_status": "DISB_REV_TIMEOUT"}, "sql": {"status": "FAILED", "dcb_status": "REVERSED_TIMEOUT", "dlp_status": ""}},
	{"csv": {"auto_adj_txn_status": `["DPP"]`, "bpp_txn_status": "", "dcb_txn_status": "REVERSED", "dpp_txn_status": "DISB_REV_CLO_REV_FAILED"}, "sql": {"status": "FAILED", "dcb_status": "", "dlp_status": "REVERSED_FAILED"}},
	{"csv": {"auto_adj_txn_status": `["DPP"]`, "bpp_txn_status": "", "dcb_txn_status": "REVERSED", "dpp_txn_status": "DISB_REV_CLO_REV_TIMEOUT"}, "sql": {"status": "FAILED", "dcb_status": "", "dlp_status": "REVERSED_TIMEOUT"}},
	{"csv": {"auto_adj_txn_status": `["DPP"]`, "bpp_txn_status": "", "dcb_txn_status": "REVERSED", "dpp_txn_status": "DISB_REV_TIMEOUT"}, "sql": {"status": "FAILED", "dcb_status": "REVERSED_TIMEOUT", "dlp_status": ""}},
	{"csv": {"auto_adj_txn_status": `["DPP"]`, "bpp_txn_status": "", "dcb_txn_status": "FAILED", "dpp_txn_status": "CLO_REV_FAILED"}, "sql": {"status": "FAILED", "dcb_status": "", "dlp_status": "REVERSED_FAILED"}},
	{"csv": {"auto_adj_txn_status": `["DPP"]`, "bpp_txn_status": "", "dcb_txn_status": "FAILED", "dpp_txn_status": "CLO_REV_TIMEOUT"}, "sql": {"status": "FAILED", "dcb_status": "", "dlp_status": "REVERSED_TIMEOUT"}},
	{"csv": {"auto_adj_txn_status": `["DLP"]`, "bpp_txn_status": "", "dcb_txn_status": "FAILED", "dpp_txn_status": "CLO_REV_TIMEOUT"}, "sql": {"status": "FAILED", "dcb_status": "", "dlp_status": "REVERSED_TIMEOUT"}},
	{"csv": {"auto_adj_txn_status": `["DPP"]`, "bpp_txn_status": "", "dcb_txn_status": "FAILED", "dpp_txn_status": "PRCS"}, "sql": {"status": "PROCESSING", "dcb_status": "", "dlp_status": ""}},
}

var recTemplate = map[string]string{
	"reconcile_status":        "DLP Unmatch",
	"unmatch_reason":          "No Record DLP",
	"oper_trigger":            "Y",
	"auto_adj_txn_status":     `["DLP","DPP"]`,
	"auto_adj_acct_status":    `["DLP","DCB"]`,
	"reconcile_ref_id":        "0006FFFFFFF-K30001FFFFFFFFFF-145318-422109005368",
	"dcb_check_duplicate_key": "0006FFFFFFF-K30001FFFFFFFFFF-145318-422109005368",
	"dlp_check_duplicate_key": "",
	"dpp_check_duplicate_key": "0006FFFFFFF-K30001FFFFFFFFFF-145318-422109005368",
	"bpp_check_duplicate_key": "0006FFFFFFF-K30001FFFFFFFFFF-145318-422109005368",
	"dlp_effective_date":      "",
	"dlp_event_dtm":           "",
	"dlp_from_loan_acct_no":   "",
	"dlp_from_loan_acct_id":   "",
	"dlp_txn_status":          "",
	"dlp_txn_amt":             "",
	"dlp_acct_status":         "",
	"dlp_product_flow_type":   "",
	"dpp_effective_date":      "2025-01-01",
	"dpp_event_dtm":           "2025-01-01 15:40:01",
	"dpp_from_loan_acct_no":   "123456789012",
	"dpp_txn_status":          "COMPLETED",
	"dpp_txn_amt":             "1000",
	"dpp_cust_fee":            "1000",
	"dpp_banking_agent_fee":   "1000",
	"dpp_dcb_status":          "REVERTED",
	"dpp_dlp_status":          "REVERTED",
	"dpp_comp_code":           "1143",
	"dpp_posting_type":        "OUTBOUND",
	"dcb_effective_date":      "2025-01-01",
	"dcb_event_dtm":           "2025-01-01 15:40:01",
	"dcb_from_loan_acct_id":   "00e41189-f298-4774-b826-03fce0ce62c9",
	"dcb_txn_status":          "SUCCESS",
	"dcb_txn_amt":             "1000",
	"dcb_cust_fee":            "1000",
	"dcb_acct_status":         "OPEN",
	"dcb_posting_type":        "OUTBOUND",
	"bpp_effective_date":      "2025-01-01",
	"bpp_event_dtm":           "2025-01-01 15:40:01",
	"bpp_txn_status":          "SUCCESS",
	"bpp_txn_amt":             "1000",
	"bpp_banking_agent_fee":   "1000",
	"bpp_comp_code":           "1143",
}

var sqlTemplate = map[string]string{
	"ref_id":                    "144878b5-1e83-4532-8208-42c07cf548da",
	"payment_token":             "490f3c02-6027-472d-863f-1be8500b94ad",
	"retrieval_ref_no":          "611209005063",
	"cust_ref_id":               "678319340991807",
	"bpp_retrieval_ref_no":      "",
	"bpp_txn_ref":               "490f3c02-6027-472d-863f-1be8500b94ad",
	"customer_note":             "",
	"from_acct_id":              "76c191ed-17e2-49a5-9cc7-68279f86f33a",
	"status":                    "COMPLETED",
	"dcb_status":                "",
	"status_cd":                 "0000",
	"status_desc":               "Success",
	"biller_ref_type":           "payeeCode",
	"biller_ref_value":          "KTC",
	"biller_name_th":            "บริษัท บัตรกรุงไทย จำกัด (มหาชน)",
	"biller_name_en":            "KRUNGTHAI CARD PUBLIC COMPANY LIMITED",
	"to_display_name":           "บริษัท บัตรกรุงไทย จำกัด (มหาชน)",
	"channel_agent_id":          "Mobile",
	"customer_fee":              "0.00",
	"company_fee":               "0.00",
	"banking_agent_fee":         "5.00",
	"total_fee":                 "5.00",
	"payment_fee":               "0.00",
	"amount":                    "10.00",
	"ref1":                      "5406040100019491",
	"ref2":                      "",
	"ref3":                      "",
	"ref4":                      "",
	"account_type":              "SA01",
	"source_of_fund":            "AC",
	"sof_type":                  `{"casa":{"sofAccount":"1300001935","sofBranchCode":"001","sofAccountName":"Teerapol Sanghangrattana","currency":"THB","deductAmount":10,"toCurrencyCode":"","convertedAmount":0,"fromCostCenter":"","exchangeRate":0}}`,
	"req_by":                    "VB",
	"req_dtm":                   "2026-04-22 09:47:28.000000",
	"reverse_dtm":               "",
	"bill_payment_workflow":     "BANKING_AGENT",
	"to_acct_no":                "",
	"to_bank_cd":                "",
	"proc_cd":                   "",
	"terminal_type":             "",
	"category":                  "",
	"created_dtm":               "2026-04-22 09:47:28.511890",
	"updated_dtm":               "2026-04-22 09:47:32.476955",
	"payment_txn_ref":           "D0260422094728305063",
	"from_acct_no":              "130000193510001",
	"pib_id":                    "019db315-edba-7f2b-9409-6f701f5495bd",
	"dcb_created_request_id":    "144878b5-1e83-4532-8208-42c07cf548da",
	"print1":                    "",
	"print2":                    "",
	"print3":                    "",
	"print4":                    "",
	"print5":                    "",
	"print6":                    "",
	"print7":                    "",
	"transaction_type":          "PAYMENT",
	"transaction_date_time":     "2026-04-22 09:47:28.000000",
	"transaction_code":          "MSBPIBN",
	"internal_account_id":       "CLEARING_WAREHOUSE",
	"transaction_class":         "D",
	"denomination":              "THB",
	"reversal_flag":             "",
	"tfr_dtm":                   "2026-04-22 09:47:32.097000",
	"fee_internal_account_id":   "",
	"fee_transaction_code":      "",
	"fee_transaction_amount":    "0.00",
	"fee_type":                  "",
	"posting_type":              "OUTBOUND",
	"effective_date":            "2026-04-22",
	"dlp_status":                "",
	"state":                     "",
	"partner_ref_id":            "",
	"from_main_account_no":      "1300001935",
	"from_address":              "DEFAULT",
	"to_main_account_no":        "",
	"to_address":                "",
	"is_force_success":          "",
	"input_terminal":            "KEYIN",
	"from_bank_code":            "088",
	"from_account_display_name": "ธีรพล แสงแห่งรัตนะ",
	"from_account_name_th":      "ธีรพล แสงแห่งรัตนะ",
	"from_account_name_en":      "Teerapol Sanghangrattana",
	"from_province_code":        "",
	"term_type":                 "",
	"pan_id":                    "",
	"terminal_id":               "",
	"transferee_fee":            "0.00",
	"transferer_fee":            "0.00",
	"sender_fee":                "0.00",
	"instruction_id":            "",
	"type_of_sender":            "",
	"type_of_receiver":          "",
	"share_flag":                "",
	"mer_cat_code":              "",
	"settlement_date":           "",
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
	csvFilename := fmt.Sprintf("DISBURSE_TO_BA_RECONCILE_UNMATCHED_%s.csv", csvDate.Format("20060102"))

	var csvRows [][]string
	sqlRowsMap := make(map[string]map[string]string)
	var sharedRefs []string

	for t := 0; t < times; t++ {
		c := cases[t%len(cases)]
		sharedRef := newUUIDv7()

			// Build CSV row
			csvRowMap := make(map[string]string)
			for k, v := range recTemplate {
				csvRowMap[k] = v
			}
			csvRowMap["reconcile_ref_id"] = sharedRef
			csvRowMap["bpp_check_duplicate_key"] = sharedRef
			csvRowMap["dpp_check_duplicate_key"] = sharedRef
			csvRowMap["dcb_check_duplicate_key"] = sharedRef
			csvRowMap["dlp_check_duplicate_key"] = sharedRef
			for k, v := range c["csv"] {
				csvRowMap[k] = v
			}
			for k, v := range csvRowMap {
				if v != "" {
					if strings.Contains(k, "dtm") {
						csvRowMap[k] = now.Format("2006-01-02 15:04:05")
					} else if strings.Contains(k, "date") {
						csvRowMap[k] = now.Format("2006-01-02")
					}
				}
			}
			var row []string
			for _, h := range csvHeader {
				row = append(row, csvRowMap[h])
			}
			csvRows = append(csvRows, row)

			// Build SQL row
			sqlVals := make(map[string]string)
			for k, v := range sqlTemplate {
				sqlVals[k] = v
			}
			sqlVals["ref_id"] = sharedRef
			sqlVals["dcb_created_request_id"] = sharedRef
			for k, v := range c["sql"] {
				sqlVals[k] = v
			}
			for k, v := range sqlVals {
				if v != "" {
					if strings.Contains(k, "dtm") {
						sqlVals[k] = now.Format("2006-01-02 15:04:05")
					} else if strings.Contains(k, "date") && len(v) == 10 {
						sqlVals[k] = now.Format("2006-01-02")
					}
				}
			}
			sqlRowsMap[sharedRef] = sqlVals
		sharedRefs = append(sharedRefs, sharedRef)
	}

	// Write raw.csv
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

	// Write state.json
	state := StateFile{
		GeneratedAt: now.Format(time.RFC3339),
		Timestamp:   timestamp,
		CsvFilename: csvFilename,
		SqlRows:     sqlRowsMap,
		SharedRefs:  sharedRefs,
	}
	stateBytes, _ := json.MarshalIndent(state, "", "  ")
	os.WriteFile(filepath.Join(workDir, "state.json"), stateBytes, 0644)

	// Update .latest
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
