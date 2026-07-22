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

func newUUIDv7() string {
	u, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return u.String()
}

func cmdGenerate(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: go run . generate <records> [types]")
		fmt.Println("  types: comma-separated subset of:")
		fmt.Println("    inbound_promptpay,inbound_actual_account,outbound_promptpay,outbound_actual_account")
		fmt.Println("  omitted = all four (default, matches a real off-us batch)")
		os.Exit(1)
	}
	times, err := strconv.Atoi(args[0])
	if err != nil || times < 1 {
		fmt.Printf("Invalid records value: %q — must be a positive integer\n", args[0])
		os.Exit(1)
	}

	var selected []string
	if len(args) > 1 {
		for _, k := range strings.Split(args[1], ",") {
			k = strings.TrimSpace(k)
			if _, ok := typeDefs[k]; !ok {
				fmt.Printf("Unknown type: %q\n", k)
				os.Exit(1)
			}
			selected = append(selected, k)
		}
	} else {
		selected = typesOrder
	}
	// keep canonical order regardless of how the user listed them
	var orderedSelected []string
	for _, k := range typesOrder {
		for _, s := range selected {
			if s == k {
				orderedSelected = append(orderedSelected, k)
				break
			}
		}
	}
	selected = orderedSelected

	now := time.Now()
	timestamp := now.Format("20060102_150405")
	workDir := filepath.Join(baseDir(), "work", timestamp)
	os.MkdirAll(workDir, os.ModePerm)

	cfg := readConfig()
	csvDate := extractDateFromBasePath(cfg.BasePath)

	typesState := make(map[string]TypeState)

	for _, key := range selected {
		def := typeDefs[key]
		sqlCols := sqlColumnsFor(def)

		csvFilename := fmt.Sprintf("%s_RECONCILE_AUTO_ADJUST_%s.csv", def.FilePrefix, csvDate.Format("20060102"))
		rawCsvFilename := fmt.Sprintf("raw_%s.csv", key)

		var csvRows [][]string
		sqlRowsMap := make(map[string]map[string]string)
		var sharedRefs []string

		for t := 0; t < times; t++ {
			sharedRef := newUUIDv7()

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
				"vfs_from_bank_code":       "008",
				"vfs_from_account_no":      fmt.Sprintf("%d", 1000000000+rand.Int63n(9000000000)),
				"vfs_from_pocket_no":       fmt.Sprintf("%d", 100000000000000+rand.Int63n(900000000000000)),
				"vfs_from_tm_account_id":   newUUIDv7(),
				"vfs_to_bank_code":         "008",
				"vfs_to_account_no":        fmt.Sprintf("%d", 1000000000+rand.Int63n(9000000000)),
				"vfs_to_pocket_no":         fmt.Sprintf("%d", 100000000000000+rand.Int63n(900000000000000)),
				"vfs_to_tm_account_id":     newUUIDv7(),
				"vfs_transaction_type":     "POCKET_TRANSFER",
				"vfs_posting_type":         "OUTBOUND_INBOUND",
				"vfs_transaction_amount":   "1000",
				"vfs_transaction_fee":      "1000",
				"vfs_channel":              "CLICX App",
				"dcb_effective_date":       now.Format("2006-01-02"),
				"dcb_transaction_date":     now.Format("2006-01-02 15:04:05"),
				"dcb_transaction_status":   "SUCCESS",
				"dcb_from_bank_code":       "008",
				"dcb_from_tm_account_id":   newUUIDv7(),
				"dcb_to_bank_code":         "008",
				"dcb_to_tm_account_id":     newUUIDv7(),
				"dcb_transaction_amount":   "1000",
				"dcb_transaction_fee":      "1000",
			}
			var row []string
			for _, h := range csvHeader {
				row = append(row, csvRowMap[h])
			}
			csvRows = append(csvRows, row)

			sqlValsRaw := make(map[string]interface{}, len(def.Template))
			for k, v := range def.Template {
				sqlValsRaw[k] = v
			}
			sqlValsRaw[def.RefColumn] = sharedRef

			switch key {
			case "outbound_promptpay", "outbound_actual_account":
				sqlValsRaw["seq_id"] = 60000000000000000 + rand.Int63n(9999999999999999)
				sqlValsRaw["req_dtm"] = now.Format("2006-01-02 15:04:05")
				sqlValsRaw["created_dtm"] = now.Format("2006-01-02 15:04:05.000")
				sqlValsRaw["updated_dtm"] = now.Format("2006-01-02 15:04:05.000")
				sqlValsRaw["retrieval_ref_no"] = newUUIDv7()
				sqlValsRaw["effective_date"] = now.Format("2006-01-02")
			case "inbound_promptpay":
				sqlValsRaw["ctfi_seq_id"] = 60000000000000000 + rand.Int63n(9999999999999999)
				sqlValsRaw["ctfi_req_dtm"] = now.Format("2006-01-02 15:04:05.000")
				sqlValsRaw["ctfi_creat_dtm"] = now.Format("2006-01-02 15:04:05.000")
				sqlValsRaw["ctfi_updat_dtm"] = now.Format("2006-01-02 15:04:05.000")
				sqlValsRaw["ctfi_eff_date"] = now.Format("2006-01-02")
				sqlValsRaw["ctfi_txn_ref_id"] = fmt.Sprintf("D%s%06d", now.Format("060102150405"), t)
				sqlValsRaw["ctfi_req_id"] = newUUIDv7()
				sqlValsRaw["ctfi_from_acct_id"] = fmt.Sprintf("%d", 1000000000+rand.Int63n(9000000000))
				sqlValsRaw["ctfi_to_acct_id"] = newUUIDv7()
				sqlValsRaw["ctfi_to_any_id"] = fmt.Sprintf("088987%09d", rand.Int63n(1000000000))
				sqlValsRaw["ctfi_sending_bank_rrn"] = fmt.Sprintf("%012d", rand.Int63n(1000000000000))
			case "inbound_actual_account":
				sqlValsRaw["acti_seq_id"] = 60000000000000000 + rand.Int63n(9999999999999999)
				sqlValsRaw["acti_req_dtm"] = now.Format("2006-01-02 15:04:05.000")
				sqlValsRaw["acti_creat_dtm"] = now.Format("2006-01-02 15:04:05.000")
				sqlValsRaw["acti_updat_dtm"] = now.Format("2006-01-02 15:04:05.000")
				sqlValsRaw["acti_eff_date"] = now.Format("2006-01-02")
				sqlValsRaw["acti_txn_ref_id"] = fmt.Sprintf("D%s%06d", now.Format("060102150405"), t)
				sqlValsRaw["acti_req_id"] = newUUIDv7()
				sqlValsRaw["acti_from_acct_id"] = newUUIDv7()
				sqlValsRaw["acti_to_acct_id"] = newUUIDv7()
			}

			sqlVals := make(map[string]string)
			for _, col := range sqlCols {
				sqlVals[col] = sqlFormat(sqlValsRaw[col])
			}
			sqlRowsMap[sharedRef] = sqlVals
			sharedRefs = append(sharedRefs, sharedRef)
		}

		rawCsvPath := filepath.Join(workDir, rawCsvFilename)
		f, err := os.Create(rawCsvPath)
		if err != nil {
			panic(err)
		}
		f.WriteString(strings.Join(csvHeader, "|") + "\n")
		for _, row := range csvRows {
			f.WriteString(strings.Join(row, "|") + "\n")
		}
		f.Close()

		typesState[key] = TypeState{
			FilePrefix:     def.FilePrefix,
			ControlSlug:    def.ControlSlug,
			Table:          def.Table,
			RefColumn:      def.RefColumn,
			StatusColumn:   def.StatusColumn,
			SqlColumns:     sqlCols,
			RawCsvFilename: rawCsvFilename,
			CsvFilename:    csvFilename,
			SqlRows:        sqlRowsMap,
			SharedRefs:     sharedRefs,
		}

		fmt.Printf("Generated %d record(s) [%s] → %s\n", times, key, rawCsvPath)
	}

	state := StateFile{
		GeneratedAt: now.Format(time.RFC3339),
		Timestamp:   timestamp,
		Types:       typesState,
	}
	stateBytes, _ := json.MarshalIndent(state, "", "  ")
	os.WriteFile(filepath.Join(workDir, "state.json"), stateBytes, 0644)
	os.WriteFile(filepath.Join(baseDir(), ".latest"), []byte(filepath.Base(workDir)+"\n"), 0644)

	fmt.Printf("\nWork dir : %s\n", workDir)
	fmt.Printf("\nEdit raw_<type>.csv if needed, then run:\n  go run . finalize\n")
}
