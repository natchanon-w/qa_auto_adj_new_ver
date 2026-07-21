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

// csvHeader mirrors CardlessDepositAdjustmentDetail in
// payment-bulk-adj-cardless-deposit-processor/app/batch/detail_parser.go
var csvHeader = []string{
	"reconcile_status", "unmatch_reason", "auto_adjust_status", "transaction_reference_id",
	"sams_check_duplicate_key", "vfs_check_duplicate_key", "dcb_check_duplicate_key",
	"sams_effective_date", "sams_transaction_date", "sams_transaction_status",
	"sams_from_account_no", "sams_to_account_no", "sams_transaction_amount", "sams_transaction_fee",
	"vfs_effective_date", "vfs_transaction_date", "vfs_transaction_status",
	"vfs_from_account_no", "vfs_from_pocket_no", "vfs_from_tm_account_id",
	"vfs_to_account_no", "vfs_to_pocket_no", "vfs_to_tm_account_id",
	"vfs_transaction_amount", "vfs_transaction_fee", "vfs_channel",
	"dcb_effective_date", "dcb_transaction_date", "dcb_transaction_status",
	"dcb_from_tm_account_id", "dcb_to_tm_account_id", "dcb_transaction_amount", "dcb_transaction_fee",
	"cardless_type",
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
	csvFilename := fmt.Sprintf("CARDLESS_RECONCILE_AUTO_ADJUST_%s.csv", csvDate.Format("20060102"))

	var csvRows [][]string
	sqlRowsMap := make(map[string]map[string]string)
	var sharedRefs []string

	dtmFull := now.Format("2006-01-02 15:04:05.000000")

	for t := 0; t < times; t++ {
		sharedRef := newUUIDv7()

		samsToAccount := fmt.Sprintf("%d", 1000000000+rand.Int63n(9000000000))
		samsAmount := fmt.Sprintf("%d", 100+rand.Intn(4901))
		vfsToAccount := fmt.Sprintf("%d", 1000000000+rand.Int63n(9000000000))
		vfsAmount := fmt.Sprintf("%d", 100+rand.Intn(4901))
		dcbAmount := fmt.Sprintf("%d", 100+rand.Intn(4901))

		vfsToPocket := newUUIDv7()
		vfsToTMAcc := newUUIDv7()
		dcbToTMAcc := newUUIDv7()

		vfsCheckDup := fmt.Sprintf("D%s%06d", now.Format("060102150405"), t)
		dcbCheckDup := fmt.Sprintf("DigitalPaymentProcessorClientIDD%s%06d", now.Format("060102150405"), t)

		csvRowMap := map[string]string{
			"reconcile_status":         "VFS unmatch",
			"unmatch_reason":           "Unmatch Status Auto adjustment",
			"auto_adjust_status":       autoAdjustStatuses[t%len(autoAdjustStatuses)],
			"transaction_reference_id": sharedRef,
			"sams_check_duplicate_key": sharedRef,
			"vfs_check_duplicate_key":  vfsCheckDup,
			"dcb_check_duplicate_key":  dcbCheckDup,
			"sams_effective_date":      now.Format("02/01/2006"),
			"sams_transaction_date":    now.Format("02/01/2006 15:04"),
			"sams_transaction_status":  "N",
			"sams_from_account_no":     "NULL",
			"sams_to_account_no":       samsToAccount,
			"sams_transaction_amount":  samsAmount,
			"sams_transaction_fee":     "10",
			"vfs_effective_date":       now.Format("02/01/2006"),
			"vfs_transaction_date":     now.Format("02/01/2006 15:04"),
			"vfs_transaction_status":   "PROCESSING",
			"vfs_from_account_no":      "NULL",
			"vfs_from_pocket_no":       "NULL",
			"vfs_from_tm_account_id":   "NULL",
			"vfs_to_account_no":        vfsToAccount,
			"vfs_to_pocket_no":         vfsToPocket,
			"vfs_to_tm_account_id":     vfsToTMAcc,
			"vfs_transaction_amount":   vfsAmount,
			"vfs_transaction_fee":      "30",
			"vfs_channel":              "CLICX app",
			"dcb_effective_date":       now.Format("02/01/2006"),
			"dcb_transaction_date":     now.Format("02/01/2006 15:04"),
			"dcb_transaction_status":   "SUCCESS",
			"dcb_from_tm_account_id":   "NULL",
			"dcb_to_tm_account_id":     dcbToTMAcc,
			"dcb_transaction_amount":   dcbAmount,
			"dcb_transaction_fee":      "10",
			"cardless_type":            "CLD",
		}

		var row []string
		for _, h := range csvHeader {
			row = append(row, csvRowMap[h])
		}
		csvRows = append(csvRows, row)

		sqlAmt := fmt.Sprintf("%d.00", 100+rand.Intn(4901))

		sqlValsRaw := map[string]interface{}{
			"id":                                newUUIDv7(),
			"req_ref_id":                        sharedRef,
			"req_dtm":                           now.Format("2006-01-02 15:04:05+07"),
			"req_channel":                       "BASE24",
			"requester":                         "VB",
			"pan_marking":                       fmt.Sprintf("XXXXXXXXXXXX%d", 1000+rand.Intn(9000)),
			"trace_id":                          fmt.Sprintf("%d", 100000+rand.Intn(900000)),
			"mti":                               "0200",
			"inteface":                          "KTB",
			"processing_code":                   "2I0010",
			"card_ref":                          fmt.Sprintf("%d", 1000000000000000+rand.Int63n(9000000000000000)),
			"terminal_id":                       fmt.Sprintf("TERMSIT%d", 100+rand.Intn(900)),
			"acquirer_id":                       "0006",
			"card_acceptor_identification_code": "MERCHANT001",
			"location":                          "000157KRUNGTHAI BANK CITYXXXXXXXXXX010TH",
			"merchant_cat_code":                 "6011",
			"rrn":                               fmt.Sprintf("%d", 100000000000+rand.Int63n(900000000000)),
			"transaction_status":                "PROCESSING",
			"cust_ref_id":                       fmt.Sprintf("%d", 100000000000000+rand.Int63n(900000000000000)),
			"to_account_no":                     fmt.Sprintf("%010d", 1000000000+rand.Int63n(9000000000)),
			"to_account_name_th":                "สมเจตน์ ไตรพัฒนาพร",
			"to_account_name_en":                "Somjet Tripattanaporn",
			"to_account_branch_code":            "00000",
			"to_account_cost_center":            "",
			"to_product_group":                  "SAV",
			"to_product_type":                   "SA01",
			"to_pocket_no":                      fmt.Sprintf("%d", 100000000000000+rand.Int63n(900000000000000)),
			"to_bank_code":                      "006",
			"to_account_type":                   "ACCOUNT",
			"to_account_class":                  "D",
			"to_account_group":                  "SAVINGS",
			"to_core_bank":                      "DCB",
			"region":                            "IntraRegion",
			"request_id":                        newUUIDv7(),
			"client_id":                         "DigitalPaymentProcessorClientID",
			"payment_transaction_ref":           fmt.Sprintf("D%s%06d", now.Format("060102150405"), t),
			"transaction_type":                  "DEPOSIT",
			"posting_type":                      "INBOUND",
			"transaction_amount":                sqlAmt,
			"denomination":                      "THB",
			"transaction_datetime":              now.Format("2006-01-02 15:04:05+07"),
			"effective_date":                    now.Format("20060102"),
			"to_account_id":                     newUUIDv7(),
			"to_address":                        "DEFAULT",
			"to_transaction_code":               "CSCLDNN",
			"from_internal_account_id":          "INTER_OFFICE_MACHINE_DEPOSIT",
			"to_transaction_class":              "D",
			"fee_internal_account_id":           "",
			"fee_transaction_code":              "",
			"fee_transaction_amount":            "0.00",
			"fee_type":                          "",
			"b24_fee":                           "0.00",
			"access_fee":                        "0.00",
			"system_fee":                        "0.00",
			"acq_fee":                           nil,
			"iss_fee":                           nil,
			"to_fee":                            nil,
			"pib_id":                            newUUIDv7(),
			"transaction_created_datetime":      now.Format("2006-01-02 15:04:05.000") + "+07",
			"transaction_created_request_id":    newUUIDv7(),
			"transaction_value_datetime":        now.Format("2006-01-02 15:04:05.000") + "+07",
			"to_account_ledger_balance":         "0.00",
			"to_account_available_balance":      "0.00",
			"dcb_resp_code":                     "0000",
			"dcb_resp_msg":                      "success",
			"created_dtm":                       dtmFull + "+07",
			"updated_dtm":                       dtmFull + "+07",
			"operation_transaction_ref":         nil,
			"location_cost_center":              nil,
			"traceparent":                       nil,
			"term_id":                           nil,
			"local_transaction_time":            nil,
			"local_transaction_date":            nil,
			"transmission_date_time":            nil,
			"cdi_token":                         nil,
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

func newUUIDv7() string {
	u, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return u.String()
}
