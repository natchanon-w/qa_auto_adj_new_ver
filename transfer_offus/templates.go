package main

// csvHeader mirrors OffUsAdjustmentDetail in
// payment-bulk-adj-transfer-offus-processor/app/batch/detail_parser.go —
// shared by all 4 file variants.
var csvHeader = []string{
	"reconcile_status", "unmatch_reason", "auto_adjust_status", "transaction_reference_id",
	"vfs_check_duplicate_key", "dcb_check_duplicate_key",
	"vfs_effective_date", "vfs_transaction_date", "vfs_transaction_status",
	"vfs_from_bank_code", "vfs_from_account_no", "vfs_from_pocket_no", "vfs_from_tm_account_id",
	"vfs_to_bank_code", "vfs_to_account_no", "vfs_to_pocket_no", "vfs_to_tm_account_id",
	"vfs_transaction_type", "vfs_posting_type",
	"vfs_transaction_amount", "vfs_transaction_fee", "vfs_channel",
	"dcb_effective_date", "dcb_transaction_date", "dcb_transaction_status",
	"dcb_from_bank_code", "dcb_from_tm_account_id",
	"dcb_to_bank_code", "dcb_to_tm_account_id",
	"dcb_transaction_amount", "dcb_transaction_fee",
}

// autoAdjustStatuses cycles through both outcomes so generated batches exercise
// both the COMPLETED and FAILED adjustment paths.
var autoAdjustStatuses = []string{"COMPLETED", "FAILED"}

// TypeDef describes one of the 4 off-us file variants: its file-name prefix
// (real production spelling — PROMPTPAY, not the processor code's PROMTPAY
// typo), its source SQL table, and that table's column template. Column
// values below mirror payment-bulk-adj-transfer-offus-processor/local-scripts/
// generate_test_data.py's TABLE_CONFIGS, translated to Go.
type TypeDef struct {
	Key          string
	FilePrefix   string
	ControlSlug  string // used for both the control-json "table" field and its filename
	Table        string
	RefColumn    string
	StatusColumn string
	Template     map[string]interface{}
}

// typesOrder fixes iteration order everywhere (generate, finalize, GUIDE)
// so output is deterministic across runs.
var typesOrder = []string{
	"inbound_promptpay",
	"inbound_actual_account",
	"outbound_promptpay",
	"outbound_actual_account",
}

var typeDefs = map[string]TypeDef{
	"outbound_promptpay": {
		Key:          "outbound_promptpay",
		FilePrefix:   "TRANSFER_OFF_US_OUTBOUND_PROMPTPAY",
		ControlSlug:  "transfer_off_us_outbound_promptpay",
		Table:        "credit_transfer",
		RefColumn:    "ref_id",
		StatusColumn: "status",
		Template: map[string]interface{}{
			"seq_id": nil, "req_channel": "VB", "requester": "VB", "ref_id": nil,
			"req_dtm": nil, "retrieval_ref_no": nil, "pib_id": nil, "created_request_id": "",
			"amount": "0.01", "payment_txn_ref": nil, "customer_note": "Load Test",
			"status": "PROCESSING", "status_code": "0000", "status_desc": "Success",
			"payment_fee": "0.00", "service_type": "OutboundPromptpayTransfer",
			"created_dtm": nil, "updated_dtm": nil, "transfer_dtm": nil, "denomination": "THB",
			"input_terminal": "KEYIN", "terminal_id": nil, "pan_id": nil,
			"ledger_amount": "885.92", "available_amount": "885.92",
			"transaction_type": "FUND_TRANSFER", "effective_date": nil, "settlement_date": nil,
			"from_branch_code": "01", "from_currency_code": "764", "from_province_code": "10",
			"from_country_code": "TH", "from_account_no": nil, "from_account_id": nil,
			"from_trans_code": "MSTOPIN", "from_internal_account_id": "SETTLEMENT_PROMPTPAY",
			"from_account_status": 0, "from_account_class": "D", "from_account_group": "SAVINGS",
			"from_account_type": "POCKET", "from_account_display_name": "สมเจตน์ ไตรพัฒนาพร", "from_account_name_th": "สมเจตน์ ไตรพัฒนาพร",
			"from_account_name_en": "Somjet Tripattanaporn", "type_of_sender": "H", "sender_tax_id": "",
			"to_any_id": nil, "to_any_id_type": "EWALLETID", "to_bank_code": "008",
			"to_account_no": nil, "to_account_name": "สมเจตน์ ไตรพัฒนาพร", "to_account_display_name": "สมเจตน์ ไตรพัฒนาพร",
			"type_of_receiver": "H", "receiver_tax_id": "", "posting_type": "OUTBOUND",
			"from_bank_code": "008", "from_product_group": "SAV", "from_product_type": "SA01",
			"from_core_bank": "DCB", "from_pocket_no": nil, "proc_cd": "481000",
			"transferee_fee": "0.00", "transferer_fee": "0.00", "sender_fee": "0.00",
			"term_type": "80", "is_force_success": "",
		},
	},
	"outbound_actual_account": {
		Key:          "outbound_actual_account",
		FilePrefix:   "TRANSFER_OFF_US_OUTBOUND_ACTUAL_ACCOUNT",
		ControlSlug:  "transfer_off_us_outbound_actual_account",
		Table:        "actual_credit_transfer",
		RefColumn:    "ref_id",
		StatusColumn: "status",
		Template: map[string]interface{}{
			"seq_id": nil, "req_channel": "VB", "requester": "VB", "ref_id": nil,
			"req_dtm": nil, "retrieval_ref_no": nil, "pib_id": nil, "created_request_id": nil,
			"amount": "0.01", "payment_txn_ref": nil, "customer_note": "Load Test",
			"status": "PROCESSING", "status_code": "0000", "status_desc": "Success",
			"payment_fee": "0.00", "service_type": "OutboundActacctTransfer",
			"created_dtm": nil, "updated_dtm": nil, "transfer_dtm": nil, "denomination": "THB",
			"input_terminal": "KEYIN", "terminal_id": nil, "pan_id": nil,
			"ledger_amount": "885.91", "available_amount": "885.91",
			"transaction_type": "FUND_TRANSFER", "effective_date": nil, "settlement_date": nil,
			"from_branch_code": "01", "from_currency_code": "764", "from_province_code": "10",
			"from_country_code": "TH", "from_account_no": nil, "from_account_id": nil,
			"from_trans_code": "MSTOAIN", "from_internal_account_id": "SETTLEMENT_PROMPTPAY",
			"from_account_status": 0, "from_account_class": "D", "from_account_group": "SAVINGS",
			"from_account_type": "POCKET", "from_account_display_name": "สมเจตน์ ไตรพัฒนาพร", "from_account_name_th": "สมเจตน์ ไตรพัฒนาพร",
			"from_account_name_en": "Somjet Tripattanaporn", "type_of_sender": "H", "sender_tax_id": "",
			"to_bank_code": "034", "to_account_no": nil, "to_account_name": "สมเจตน์ ไตรพัฒนาพร",
			"to_account_display_name": "สมเจตน์ ไตรพัฒนาพร", "type_of_receiver": "H", "receiver_tax_id": "",
			"posting_type": "OUTBOUND", "from_bank_code": "008", "from_product_group": "SAV",
			"from_product_type": "SA01", "from_core_bank": "DCB", "from_pocket_no": nil,
			"proc_cd": "481000", "transferee_fee": "0.00", "transferer_fee": "0.00",
			"sender_fee": "0.00", "term_type": "80", "is_force_success": "",
		},
	},
	"inbound_promptpay": {
		Key:          "inbound_promptpay",
		FilePrefix:   "TRANSFER_OFF_US_INBOUND_PROMPTPAY",
		ControlSlug:  "transfer_off_us_inbound_promptpay",
		Table:        "credit_transfer_inbound",
		RefColumn:    "ctfi_tfr_ref_no",
		StatusColumn: "ctfi_status",
		Template: map[string]interface{}{
			"ctfi_seq_id": nil, "ctfi_txn_ref_id": nil, "ctfi_req_dtm": nil, "ctfi_req_id": nil,
			"ctfi_from_acct_id": nil, "ctfi_from_acct_bank_code": "014", "ctfi_from_acct_name": "สมเจตน์ ไตรพัฒนาพร",
			"ctfi_from_display_name": "สมเจตน์ ไตรพัฒนาพร", "ctfi_to_any_id": nil, "ctfi_to_any_type": "EWALLETID",
			"ctfi_to_acct_id": nil, "ctfi_to_acct_status": "0", "ctfi_to_acct_name": "สมเจตน์ ไตรพัฒนาพร",
			"ctfi_to_display_name": "สมเจตน์ ไตรพัฒนาพร", "ctfi_tfr_amt": "0.01", "ctfi_tfr_ref_no": nil,
			"ctfi_status": "PROCESSING", "ctfi_status_cd": nil, "ctfi_status_desc": nil,
			"ctfi_creat_dtm": nil, "ctfi_updat_dtm": nil, "ctfi_comments": "",
			"ctfi_sending_bank_rrn": nil, "ctfi_transmission_dtm": "1228030239",
			"ctfi_system_trace_no": "019536", "ctfi_sending_id": "014",
			"ctfi_switching_fee_amt": "0.00", "ctfi_sending_fee_amt": "0.00", "ctfi_terminal_id": nil,
			"ctfi_from_branch_cd": "014", "ctfi_terminal_type": "60", "ctfi_pan_id": nil,
			"ctfi_recipt_no": "108172", "ctfi_trans_time": "100248", "ctfi_location_cd": "0",
			"ctfi_msg_type": "220", "ctfi_processing_cd": "481000", "ctfi_from_tax_id": nil,
			"ctfi_lookup_server_name": "payment-inbound-j-promptpay-lookup-consumer",
			"ctfi_tfr_server_name": "payment-inbound-j-promptpay-transfer-consumer",
			"ctfi_mq_server_name": "payment-inbound-j-promptpay-transfer-processor-consumer",
			"ctfi_to_trans_code": "MSTIPNN", "ctfi_to_internal_account_id": "SETTLEMENT_PROMPTPAY",
			"ctfi_transaction_type": "FUND_TRANSFER", "ctfi_to_acct_bank_code": "008",
			"ctfi_to_account_class": "D", "ctfi_to_account_group": "WALLET",
			"ctfi_to_account_type": "ACCOUNT", "ctfi_to_product_group": "SAV",
			"ctfi_to_product_type": "SA01", "ctfi_to_core_bank": "DCB", "ctfi_to_branch_cd": nil,
			"ctfi_posting_type": "INBOUND", "ctfi_to_acct_no": nil, "ctfi_eff_date": nil,
			"ctfi_transfer_dtm": nil, "ctfi_to_pocket_no": nil,
		},
	},
	"inbound_actual_account": {
		Key:          "inbound_actual_account",
		FilePrefix:   "TRANSFER_OFF_US_INBOUND_ACTUAL_ACCOUNT",
		ControlSlug:  "transfer_off_us_inbound_actual_account",
		Table:        "actual_credit_transfer_inbound",
		RefColumn:    "acti_tfr_ref_no",
		StatusColumn: "acti_status",
		Template: map[string]interface{}{
			"acti_seq_id": nil, "acti_txn_ref_id": nil, "acti_req_dtm": nil, "acti_req_id": nil,
			"acti_from_acct_id": nil, "acti_from_acct_bank_code": "025", "acti_from_acct_name": "สมเจตน์ ไตรพัฒนาพร",
			"acti_from_display_name": "สมเจตน์ ไตรพัฒนาพร", "acti_to_acct_id": nil, "acti_to_acct_status": "0",
			"acti_to_acct_name": "สมเจตน์ ไตรพัฒนาพร", "acti_to_display_name": "สมเจตน์ ไตรพัฒนาพร", "acti_tfr_amt": "10.01",
			"acti_tfr_ref_no": nil, "acti_status": "PROCESSING", "acti_status_cd": "DCB410023",
			"acti_status_desc": "Breach Terms and Conditions In Smart Contract",
			"acti_creat_dtm": nil, "acti_updat_dtm": nil, "acti_comments": "",
			"acti_sending_bank_rrn": nil, "acti_transmission_dtm": "1228030239",
			"acti_system_trace_no": "019536", "acti_sending_id": "014",
			"acti_switching_fee_amt": "0.00", "acti_sending_fee_amt": "0.00", "acti_terminal_id": nil,
			"acti_from_branch_cd": "014", "acti_terminal_type": "60", "acti_pan_id": nil,
			"acti_recipt_no": "108172", "acti_trans_time": "100248", "acti_location_cd": "0",
			"acti_msg_type": "220", "acti_processing_cd": "481000", "acti_from_tax_id": nil,
			"acti_eff_date": nil, "acti_settlement_date": "1226", "acti_sender_fee_amt": "0.00",
			"acti_transferer_fee_amt": "0.00", "acti_transferee_fee_amt": "0.00",
			"acti_transfer_dtm": nil, "acti_to_cif_no": nil, "acti_receiver_tax_id": nil,
			"acti_to_branch_code": nil,
			"acti_lookup_server_name":  "payment-inbound-j-actacct-lookup-consumer",
			"acti_tfr_server_name":     "payment-inbound-j-actacct-transfer-consumer",
			"acti_mq_server_name":      "payment-inbound-j-actacct-transfer-processor-consumer",
			"acti_type_of_receiver": "H", "acti_type_of_sender": "H", "acti_to_trans_code": "MSTIANN",
			"acti_to_internal_account_id": "SETTLEMENT_PROMPTPAY", "acti_transaction_type": "FUND_TRANSFER",
			"acti_to_account_class": "D", "acti_to_account_group": "SAVINGS",
			"acti_to_account_type": "ACCOUNT", "acti_to_acct_bank_code": "008",
			"acti_to_product_group": "SAV", "acti_to_product_type": "SA01", "acti_to_core_bank": "DCB",
			"acti_posting_type": "INBOUND", "acti_to_acct_no": nil, "acti_to_pocket_no": nil,
		},
	},
}

// sqlColumnsFor returns a stable column order for a template (Go map iteration
// order is random, so the order must be pinned once and reused for both the
// INSERT header and every VALUES tuple).
func sqlColumnsFor(def TypeDef) []string {
	switch def.Key {
	case "outbound_promptpay":
		return []string{
			"seq_id", "req_channel", "requester", "ref_id", "req_dtm", "retrieval_ref_no", "pib_id",
			"created_request_id", "amount", "payment_txn_ref", "customer_note", "status", "status_code",
			"status_desc", "payment_fee", "service_type", "created_dtm", "updated_dtm", "transfer_dtm",
			"denomination", "input_terminal", "terminal_id", "pan_id", "ledger_amount", "available_amount",
			"transaction_type", "effective_date", "settlement_date", "from_branch_code", "from_currency_code",
			"from_province_code", "from_country_code", "from_account_no", "from_account_id", "from_trans_code",
			"from_internal_account_id", "from_account_status", "from_account_class", "from_account_group",
			"from_account_type", "from_account_display_name", "from_account_name_th", "from_account_name_en",
			"type_of_sender", "sender_tax_id", "to_any_id", "to_any_id_type", "to_bank_code", "to_account_no",
			"to_account_name", "to_account_display_name", "type_of_receiver", "receiver_tax_id", "posting_type",
			"from_bank_code", "from_product_group", "from_product_type", "from_core_bank", "from_pocket_no",
			"proc_cd", "transferee_fee", "transferer_fee", "sender_fee", "term_type", "is_force_success",
		}
	case "outbound_actual_account":
		return []string{
			"seq_id", "req_channel", "requester", "ref_id", "req_dtm", "retrieval_ref_no", "pib_id",
			"created_request_id", "amount", "payment_txn_ref", "customer_note", "status", "status_code",
			"status_desc", "payment_fee", "service_type", "created_dtm", "updated_dtm", "transfer_dtm",
			"denomination", "input_terminal", "terminal_id", "pan_id", "ledger_amount", "available_amount",
			"transaction_type", "effective_date", "settlement_date", "from_branch_code", "from_currency_code",
			"from_province_code", "from_country_code", "from_account_no", "from_account_id", "from_trans_code",
			"from_internal_account_id", "from_account_status", "from_account_class", "from_account_group",
			"from_account_type", "from_account_display_name", "from_account_name_th", "from_account_name_en",
			"type_of_sender", "sender_tax_id", "to_bank_code", "to_account_no", "to_account_name",
			"to_account_display_name", "type_of_receiver", "receiver_tax_id", "posting_type", "from_bank_code",
			"from_product_group", "from_product_type", "from_core_bank", "from_pocket_no", "proc_cd",
			"transferee_fee", "transferer_fee", "sender_fee", "term_type", "is_force_success",
		}
	case "inbound_promptpay":
		return []string{
			"ctfi_seq_id", "ctfi_txn_ref_id", "ctfi_req_dtm", "ctfi_req_id", "ctfi_from_acct_id",
			"ctfi_from_acct_bank_code", "ctfi_from_acct_name", "ctfi_from_display_name", "ctfi_to_any_id",
			"ctfi_to_any_type", "ctfi_to_acct_id", "ctfi_to_acct_status", "ctfi_to_acct_name",
			"ctfi_to_display_name", "ctfi_tfr_amt", "ctfi_tfr_ref_no", "ctfi_status", "ctfi_status_cd",
			"ctfi_status_desc", "ctfi_creat_dtm", "ctfi_updat_dtm", "ctfi_comments", "ctfi_sending_bank_rrn",
			"ctfi_transmission_dtm", "ctfi_system_trace_no", "ctfi_sending_id", "ctfi_switching_fee_amt",
			"ctfi_sending_fee_amt", "ctfi_terminal_id", "ctfi_from_branch_cd", "ctfi_terminal_type",
			"ctfi_pan_id", "ctfi_recipt_no", "ctfi_trans_time", "ctfi_location_cd", "ctfi_msg_type",
			"ctfi_processing_cd", "ctfi_from_tax_id", "ctfi_lookup_server_name", "ctfi_tfr_server_name",
			"ctfi_mq_server_name", "ctfi_to_trans_code", "ctfi_to_internal_account_id",
			"ctfi_transaction_type", "ctfi_to_acct_bank_code", "ctfi_to_account_class",
			"ctfi_to_account_group", "ctfi_to_account_type", "ctfi_to_product_group", "ctfi_to_product_type",
			"ctfi_to_core_bank", "ctfi_to_branch_cd", "ctfi_posting_type", "ctfi_to_acct_no",
			"ctfi_eff_date", "ctfi_transfer_dtm", "ctfi_to_pocket_no",
		}
	case "inbound_actual_account":
		return []string{
			"acti_seq_id", "acti_txn_ref_id", "acti_req_dtm", "acti_req_id", "acti_from_acct_id",
			"acti_from_acct_bank_code", "acti_from_acct_name", "acti_from_display_name", "acti_to_acct_id",
			"acti_to_acct_status", "acti_to_acct_name", "acti_to_display_name", "acti_tfr_amt",
			"acti_tfr_ref_no", "acti_status", "acti_status_cd", "acti_status_desc", "acti_creat_dtm",
			"acti_updat_dtm", "acti_comments", "acti_sending_bank_rrn", "acti_transmission_dtm",
			"acti_system_trace_no", "acti_sending_id", "acti_switching_fee_amt", "acti_sending_fee_amt",
			"acti_terminal_id", "acti_from_branch_cd", "acti_terminal_type", "acti_pan_id",
			"acti_recipt_no", "acti_trans_time", "acti_location_cd", "acti_msg_type", "acti_processing_cd",
			"acti_from_tax_id", "acti_eff_date", "acti_settlement_date", "acti_sender_fee_amt",
			"acti_transferer_fee_amt", "acti_transferee_fee_amt", "acti_transfer_dtm", "acti_to_cif_no",
			"acti_receiver_tax_id", "acti_to_branch_code", "acti_lookup_server_name",
			"acti_tfr_server_name", "acti_mq_server_name", "acti_type_of_receiver", "acti_type_of_sender",
			"acti_to_trans_code", "acti_to_internal_account_id", "acti_transaction_type",
			"acti_to_account_class", "acti_to_account_group", "acti_to_account_type",
			"acti_to_acct_bank_code", "acti_to_product_group", "acti_to_product_type", "acti_to_core_bank",
			"acti_posting_type", "acti_to_acct_no", "acti_to_pocket_no",
		}
	}
	panic("unknown type key: " + def.Key)
}
