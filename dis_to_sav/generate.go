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
	"dlp_effective_date", "dlp_event_dtm", "dlp_from_loan_acct_no", "dlp_from_loan_acct_id", "dlp_to_acct_no",
	"dlp_txn_status", "dlp_txn_amt", "dlp_acct_status", "dpp_effective_date", "dpp_event_dtm",
	"dpp_from_loan_acct_no", "dpp_from_loan_acct_id", "dpp_to_acct_no", "dpp_to_acct_id", "dpp_txn_status",
	"dpp_txn_amt", "dpp_posting_type", "dcb_effective_date", "dcb_event_dtm", "dcb_from_loan_acct_id",
	"dcb_to_acct_id", "dcb_txn_status", "dcb_txn_amt", "dcb_acct_status", "dcb_posting_type",
}

var cases = []map[string]map[string]string{
	{"csv": {"auto_adj_txn_status": `["DPP"]`, "dcb_txn_status": "SUCCESS", "dlp_txn_status": "PRCS", "dpp_txn_status": "PRCS"}, "sql": {"status": "PROCESSING"}},
	{"csv": {"auto_adj_txn_status": `["DLP"]`, "dcb_txn_status": "SUCCESS", "dlp_txn_status": "PRCS", "dpp_txn_status": "PRCS"}, "sql": {"status": "PROCESSING"}},
	{"csv": {"auto_adj_txn_status": `["DPP"]`, "dcb_txn_status": "SUCCESS", "dlp_txn_status": "PRCS", "dpp_txn_status": "CMPLT"}, "sql": {"status": "COMPLETED"}},
}

var recTemplate = map[string]string{
	"reconcile_status":         "DLP Unmatch",
	"unmatch_reason":           "No Record DLP",
	"oper_trigger":             "Y",
	"auto_adj_txn_status":      `["DLP","DPP"]`,
	"auto_adj_acct_status":     `["DLP","DCB"]`,
	"reconcile_ref_id":         "0006FFFFFFF-K30001FFFFFFFFFF-145318-422109005368",
	"dcb_check_duplicate_key":  "D0250827143812900006",
	"dlp_check_duplicate_key":  "",
	"dpp_check_duplicate_key":  "0006FFFFFFF-K30001FFFFFFFFFF-145318-422109005368",
	"dlp_effective_date":       "",
	"dlp_event_dtm":            "",
	"dlp_from_loan_acct_no":    "",
	"dlp_from_loan_acct_id":    "",
	"dlp_to_acct_no":           "",
	"dlp_txn_status":           "",
	"dlp_txn_amt":              "",
	"dlp_acct_status":          "",
	"dpp_effective_date":       "2025-01-01",
	"dpp_event_dtm":            "2025-01-01 15:40:01",
	"dpp_from_loan_acct_no":    "123456789012",
	"dpp_from_loan_acct_id":    "00e41189-f298-4774-b826-03fce0ce62c9",
	"dpp_to_acct_no":           "1234567890",
	"dpp_to_acct_id":           "11e41189-f298-4774-b826-03fce0ce62c9",
	"dpp_txn_status":           "COMPLETED",
	"dpp_txn_amt":              "1000",
	"dpp_posting_type":         "OUTBOUND_INBOUND",
	"dcb_effective_date":       "2025-01-01",
	"dcb_event_dtm":            "2025-01-01 15:40:01",
	"dcb_from_loan_acct_id":    "00e41189-f298-4774-b826-03fce0ce62c9",
	"dcb_to_acct_id":           "11e41189-f298-4774-b826-03fce0ce62c9",
	"dcb_txn_status":           "SUCCESS",
	"dcb_txn_amt":              "1000",
	"dcb_acct_status":          "OPEN",
	"dcb_posting_type":         "OUTBOUND_INBOUND",
}

var sqlTemplate = map[string]string{
	"id":                        "c4e4e67d-ee00-45f1-9d6a-85e9f5b4b858",
	"ref_id":                    "5b0232a3-236c-4b97-883d-7e5c14890fd1",
	"original_ref_id":           "",
	"req_channel":               "VB",
	"requester":                 "DLP",
	"req_dtm":                   "2026-04-22 04:48:50.000000 +00:00",
	"ref_no":                    "611211000005",
	"payment_txn_ref":           "D06L2611211485180005",
	"tfr_dtm":                   "2026-04-22 04:48:51.189000 +00:00",
	"created_request_id":        "5b0232a3-236c-4b97-883d-7e5c14890fd1",
	"amount":                    "5000.00",
	"denomination":              "THB",
	"customer_note":             "test VB drawdown to saving",
	"customer_ref_id":           "678789827700484",
	"from_main_account_id":      "",
	"from_main_account_no":      "2FDNvx3Z0fgSPDuNs83myWNKneovGhCkqyKT0TSGAvbhMUAwB/Evmg==",
	"from_transfer_account_no":  "",
	"from_acct_no":              "eBwvZ44+soewc3ewo1+sx8pc4TnP/iL2HNROhSWnAQkN/2e7uFn7QJqlPmw=",
	"from_acct_id":              "cf57d7f1-79c1-4df4-aab9-eb62be6d3fb6",
	"to_internal_acct_id":       "FUND_TRANSFER",
	"from_trans_code":           "MLDSIN",
	"from_address":              "PRINCIPAL",
	"from_acct_status":          "",
	"from_product_class":        "L",
	"from_product_group":        "LENDING",
	"from_product_type":         "LOAN",
	"from_acct_type":            "LOAN",
	"from_branch_code":          "",
	"from_account_display_name": "wLGWH3Jot9kPWKs/66G+us6GvFjgNSVDLQQC0HP1Ez8FmMTDBolcLzgwbjf5JG2+VgtqEwXy144TtfHY1/w=",
	"from_account_name_th":      "IJ2If6k9DyA7GDqaVriwH2hfNKpmO59j+J3yB6RnKfPZ+gR9f5G+gSkItu8YoVZsNDjrozTL0RmfB2pffI4=",
	"from_account_name_en":      "JB2micsdOe1X7mxj/6dxsXI9koQrJQpRIe+2k3aEQTATm9JJ5imNdnU=",
	"from_bank_code":            "088",
	"from_core_bank_channel":    "DCB",
	"from_cif_no":               "678789827700484",
	"from_cdi_token":            "",
	"from_internal_acct_id":     "FUND_TRANSFER",
	"to_main_account_id":        "",
	"to_main_account_no":        "",
	"to_transfer_account_no":    "130000315610001",
	"to_acct_no":                "QR7GI9MqYbnR4u7pvEaris9Fx1dsyjuAcAKDK1/uCfm2yWQ8bs=",
	"to_acct_id":                "39cdfce5-52af-49d8-b91e-ac14d530843d",
	"to_trans_code":             "MSADTSN",
	"to_address":                "",
	"to_acct_status":            "0",
	"to_product_class":          "D",
	"to_product_group":          "SAV",
	"to_product_type":           "SA01",
	"to_acct_type":              "POCKET",
	"to_branch_code":            "47",
	"to_account_display_name":   "wLGWH3Jot9kPWKs/66G+us6GvFjgNSVDLQQC0HP1Ez8FmMTDBolcLzgwbjf5JG2+VgtqEwXy144TtfHY1/w=",
	"to_account_name_th":        "IJ2If6k9DyA7GDqaVriwH2hfNKpmO59j+J3yB6RnKfPZ+gR9f5G+gSkItu8YoVZsNDjrozTL0RmfB2pffI4=",
	"to_account_name_en":        "JB2micsdOe1X7mxj/6dxsXI9koQrJQpRIe+2k3aEQTATm9JJ5imNdnU=",
	"to_bank_code":              "088",
	"to_core_bank_channel":      "DCB",
	"to_cif_no":                 "678789827700484",
	"to_cdi_token":              "",
	"fee_amount":                "0.00",
	"fee_internal_acct_id":      "",
	"fee_trans_code":            "",
	"fee_type":                  "",
	"status":                    "COMPLETED",
	"status_code":               "0000",
	"status_desc":               "Success",
	"debit_status":              "",
	"credit_status":             "",
	"service_type":              "Disbursement",
	"created_dtm":               "2026-04-22 04:48:51.344747 +00:00",
	"updated_dtm":               "2026-04-22 04:48:51.344747 +00:00",
	"transaction_type":          "DISBURSEMENT",
	"posting_type":              "OUTBOUND_INBOUND",
	"effective_date":            "2026-04-22",
	"reversal_flag":             "",
	"transfer_type":             "",
	"input_terminal":            "",
	"dcb_request":               `{"inbound": {"toAccountId": "39cdfce5-52af-49d8-b91e-ac14d530843d", "transactionCode": "MSADTSN", "transactionClass": "D", "internalAccountId": "FUND_TRANSFER"}, "clientId": "DigitalPaymentProcessorClientID", "outbound": {"feeType": "", "fromAddress": "PRINCIPAL", "fromAccountId": "cf57d7f1-79c1-4df4-aab9-eb62be6d3fb6", "transactionCode": "MLDSIN", "transactionClass": "L", "internalAccountId": "FUND_TRANSFER"}, "requestId": "5b0232a3-236c-4b97-883d-7e5c14890fd1", "postingType": "OUTBOUND_INBOUND", "denomination": "THB", "reversalFlag": false, "effectiveDate": "20260422", "additionalInfo": {"refId": "5b0232a3-236c-4b97-883d-7e5c14890fd1", "refNo": "611211000005", "originalChannelRequestId": "5b0232a3-236c-4b97-883d-7e5c14890fd1"}, "transactionType": "DISBURSEMENT", "transactionRefId": "D06L2611211485180005", "transactionAmount": 5000.00, "transactionDatetime": "2026-04-22T11:48:50+07:00"}`,
	"dcb_response":              `{"code": "0000", "data": {"pibId": "019db384-ff8c-7e05-b282-661dbf69b327", "valueDatetime": "2026-04-22T11:48:51.189+07:00", "additionalinfo": {"refId": "5b0232a3-236c-4b97-883d-7e5c14890fd1", "refNo": "611211000005", "txn_ref_id": "txn_D06L2611211485180005", "reversalFlag": "N", "originalChannelRequestId": "5b0232a3-236c-4b97-883d-7e5c14890fd1"}, "createdDatetime": "2026-04-22T11:48:51.189+07:00", "createdRequestId": "5b0232a3-236c-4b97-883d-7e5c14890fd1", "toAccountLedgerBalance": null, "fromAccountLedgerBalance": 0, "toAccountAvailableBalance": null, "fromAccountAvailableBalance": 0}, "message": "Success"}`,
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
	csvFilename := fmt.Sprintf("DISBURSE_TO_SAVING_RECONCILE_UNMATCHED_%s.csv", csvDate.Format("20060102"))

	var csvRows [][]string
	sqlRowsMap := make(map[string]map[string]string)
	var sharedRefs []string

	for t := 0; t < times; t++ {
		c := cases[t%len(cases)]
		sharedRef := newUUIDv7()

		csvRowMap := make(map[string]string)
		for k, v := range recTemplate {
			csvRowMap[k] = v
		}
		csvRowMap["reconcile_ref_id"] = sharedRef
		csvRowMap["dcb_check_duplicate_key"] = sharedRef
		csvRowMap["dpp_check_duplicate_key"] = sharedRef
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

		sqlVals := make(map[string]string)
		for k, v := range sqlTemplate {
			sqlVals[k] = v
		}
		sqlVals["ref_id"] = sharedRef
		sqlVals["original_ref_id"] = sharedRef
		sqlVals["id"] = newUUIDv7()
		sqlVals["created_request_id"] = sharedRef
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
