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

// piiColumns are, per type, the columns the online path stores AES-GCM
// encrypted (payment-lib-j-common-service TransferEncryptionService). The
// auto-adjustment consumers decrypt them before building the TM / CLOG message,
// so a plaintext value there fails the send. Outbound decrypt has no blank or
// gcm guard (decryptCreditTransfer / decryptActualCreditTransfer): every
// column listed for outbound must hold a non-blank ciphertext.
var piiColumns = map[string][]string{
	"inbound_actual_account": { // encryptActualCreditTransferInbound
		"acti_from_acct_id", // entity field actiFromAccountNo: the sender's account number
		"acti_from_acct_name", "acti_from_display_name", "acti_from_tax_id",
		"acti_to_acct_no", "acti_to_acct_name", "acti_to_display_name",
		"acti_to_cif_no", "acti_receiver_tax_id", "acti_to_pocket_no",
	},
	"inbound_promptpay": { // savedb AdjInboundPromptPayProduceMessageServiceImpl.decryptCreditTransferInbound
		"ctfi_from_acct_id", // entity field ctfiFromAccountNo
		"ctfi_from_acct_name", "ctfi_from_display_name", "ctfi_from_tax_id",
		"ctfi_to_any_id", "ctfi_to_acct_name", "ctfi_to_display_name",
		"ctfi_to_acct_no", "ctfi_to_pocket_no",
	},
	"outbound_promptpay": { // encryptCreditTransfer
		"from_account_no", "from_pocket_no", "from_account_name_th", "from_account_name_en",
		"from_account_display_name", "sender_tax_id",
		"to_any_id", "to_account_no", "to_account_name", "to_account_display_name", "receiver_tax_id",
	},
	"outbound_actual_account": { // encryptActualCreditTransfer
		"from_account_no", "from_pocket_no", "from_account_name_th", "from_account_name_en",
		"from_account_display_name", "sender_tax_id",
		"to_account_no", "to_account_name", "to_account_display_name", "receiver_tax_id",
	},
}

func cmdGenerate(args []string) {
	plain := false
	var positional []string
	for _, a := range args {
		if a == "--plain" {
			plain = true
			continue
		}
		positional = append(positional, a)
	}
	args = positional

	if len(args) < 1 {
		fmt.Println("Usage: go run . generate <records> [types] [--plain]")
		fmt.Println("  types: comma-separated subset of:")
		fmt.Println("    inbound_promptpay,inbound_actual_account,outbound_promptpay,outbound_actual_account")
		fmt.Println("  omitted = all four (default, matches a real off-us batch)")
		fmt.Println("  PII columns of all four types are AES-GCM encrypted with the env's key;")
		fmt.Println("  --plain skips that (inbound only, for an env with aes.gcm.enable=false —")
		fmt.Println("  outbound consumers always decrypt, so --plain breaks their TM / CLOG send)")
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

	cfg := readConfig()
	fmt.Printf("Env     : %s (bucket %s)\n", cfg.Env, cfg.Bucket)

	encryptPII := func(s string) string { return s }
	for _, k := range selected {
		if len(piiColumns[k]) > 0 {
			encryptPII = newPIIEncrypter(plain, cfg)
			break
		}
	}

	now := time.Now()
	timestamp := now.Format("20060102_150405")
	workDir := filepath.Join(baseDir(), "work", timestamp)
	os.MkdirAll(workDir, os.ModePerm)

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
				// 20-char reference: must fit varchar(20) and match across
				// transaction_reference_id (CSV) and payment_txn_ref /
				// ctfi_txn_ref_id / acti_txn_ref_id (SQL insert).
				sharedRef := fmt.Sprintf("D%s%07d", now.Format("060102150405"), t)

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
				sqlValsRaw["payment_txn_ref"] = sharedRef // what the auto-adj API matches on
				sqlValsRaw["ref_id"] = newUUIDv7()        // request id → TM requestId
				// online: account, pocket and TM account id are the same values the CSV row carries.
				sqlValsRaw["from_account_no"] = csvRowMap["vfs_from_account_no"] // TM "TR fr <no>"
				sqlValsRaw["from_pocket_no"] = csvRowMap["vfs_from_pocket_no"]   // TM fromAccountNo
				sqlValsRaw["from_account_id"] = csvRowMap["vfs_from_tm_account_id"]
				sqlValsRaw["to_account_no"] = csvRowMap["vfs_to_account_no"]
				sqlValsRaw["sender_tax_id"] = fmt.Sprintf("%013d", rand.Int63n(10000000000000))
				sqlValsRaw["receiver_tax_id"] = fmt.Sprintf("%013d", rand.Int63n(10000000000000))
				sqlValsRaw["transfer_dtm"] = now.Format("2006-01-02 15:04:05.000") // TM transactionDateTime
				if key == "outbound_promptpay" {
					sqlValsRaw["to_any_id"] = fmt.Sprintf("088987%09d", rand.Int63n(1000000000))
				}
			case "inbound_promptpay":
				sqlValsRaw["ctfi_seq_id"] = 60000000000000000 + rand.Int63n(9999999999999999)
				sqlValsRaw["ctfi_req_dtm"] = now.Format("2006-01-02 15:04:05.000")
				sqlValsRaw["ctfi_creat_dtm"] = now.Format("2006-01-02 15:04:05.000")
				sqlValsRaw["ctfi_updat_dtm"] = now.Format("2006-01-02 15:04:05.000")
				sqlValsRaw["ctfi_eff_date"] = now.Format("2006-01-02")
				sqlValsRaw["ctfi_txn_ref_id"] = sharedRef
				sqlValsRaw["ctfi_req_id"] = newUUIDv7()
				sqlValsRaw["ctfi_to_any_id"] = fmt.Sprintf("088987%09d", rand.Int63n(1000000000))
				sqlValsRaw["ctfi_sending_bank_rrn"] = fmt.Sprintf("%012d", rand.Int63n(1000000000000))
				// online: what the lookup / transfer / processor consumers write. Account, pocket and TM
				// account id are the same values the CSV row carries, so file and DB agree.
				sqlValsRaw["ctfi_from_acct_id"] = csvRowMap["vfs_from_account_no"] // sender account no
				sqlValsRaw["ctfi_to_acct_id"] = csvRowMap["vfs_to_tm_account_id"]  // lookup accountId → TM toAccountId
				sqlValsRaw["ctfi_to_acct_no"] = csvRowMap["vfs_to_account_no"]     // lookup accountNo → TM "TR to <no>"
				sqlValsRaw["ctfi_to_pocket_no"] = csvRowMap["vfs_to_pocket_no"]    // lookup pocketNumber → TM toAccountNo
				sqlValsRaw["ctfi_from_tax_id"] = fmt.Sprintf("%013d", rand.Int63n(10000000000000))
				sqlValsRaw["ctfi_transfer_dtm"] = now.Format("2006-01-02 15:04:05.000") // DCB createdDatetime → TM transactionDateTime
				sqlValsRaw["ctfi_tfr_ref_no"] = fmt.Sprintf("%020d", rand.Int63n(1<<62))  // DCB pibId (varchar 20)
			case "inbound_actual_account":
				sqlValsRaw["acti_seq_id"] = 60000000000000000 + rand.Int63n(9999999999999999)
				sqlValsRaw["acti_req_dtm"] = now.Format("2006-01-02 15:04:05.000")
				sqlValsRaw["acti_creat_dtm"] = now.Format("2006-01-02 15:04:05.000")
				sqlValsRaw["acti_updat_dtm"] = now.Format("2006-01-02 15:04:05.000")
				sqlValsRaw["acti_eff_date"] = now.Format("2006-01-02")
				sqlValsRaw["acti_txn_ref_id"] = sharedRef
				sqlValsRaw["acti_req_id"] = newUUIDv7()
				sqlValsRaw["acti_sending_bank_rrn"] = fmt.Sprintf("%012d", rand.Int63n(1000000000000))
				// online: what the lookup / transfer / processor consumers write. Account, pocket and TM
				// account id are the same values the CSV row carries, so file and DB agree.
				sqlValsRaw["acti_from_acct_id"] = csvRowMap["vfs_from_account_no"] // sender account no (ITMX fromAcctID)
				sqlValsRaw["acti_to_acct_id"] = csvRowMap["vfs_to_tm_account_id"]  // TM account id (lookup accountId)
				sqlValsRaw["acti_to_acct_no"] = csvRowMap["vfs_to_account_no"]     // ITMX toAcctID → TM "TR to <no>"
				sqlValsRaw["acti_to_pocket_no"] = csvRowMap["vfs_to_pocket_no"]    // lookup pocketNumber → TM toAccountNo
				sqlValsRaw["acti_to_cif_no"] = fmt.Sprintf("%015d", rand.Int63n(1000000000000000)) // lookup custRefId → TM toCustomerId
				sqlValsRaw["acti_from_tax_id"] = fmt.Sprintf("%013d", rand.Int63n(10000000000000))
				sqlValsRaw["acti_receiver_tax_id"] = fmt.Sprintf("%013d", rand.Int63n(10000000000000))
				sqlValsRaw["acti_transfer_dtm"] = now.Format("2006-01-02 15:04:05.000") // DCB valueDatetime → TM transactionDateTime
				sqlValsRaw["acti_tfr_ref_no"] = newUUIDv7()                              // DCB pibId
			}
			for _, col := range piiColumns[key] {
				if s, ok := sqlValsRaw[col].(string); ok {
					sqlValsRaw[col] = encryptPII(s)
				}
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
			ResetStatus:    def.ResetStatus,
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
