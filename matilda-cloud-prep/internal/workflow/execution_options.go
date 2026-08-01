package workflow

import (
	"fmt"
	"regexp"
	"strings"
)

const ExecutionOptionsSchemaVersion = "matilda_cloud_prep.execution_options_v0"
const DefaultExecutionTimeoutSeconds = 300

type InterfaceMode string

const (
	InterfaceModeDirect InterfaceMode = "direct"
	InterfaceModeGuided InterfaceMode = "guided"
)

type ExecutionOptions struct {
	SchemaVersion  string              `json:"schema_version"`
	InterfaceMode  InterfaceMode       `json:"interface_mode"`
	TimeoutSeconds int                 `json:"timeout_seconds"`
	Selectors      *ExecutionSelectors `json:"selectors,omitempty"`
	Approvals      []ExecutionApproval `json:"approvals,omitempty"`
}

type ExecutionSelectors struct {
	AWS *AWSExecutionSelectors `json:"aws,omitempty"`
}

type AWSExecutionSelectors struct {
	Profile       string `json:"profile,omitempty"`
	Region        string `json:"region,omitempty"`
	CUR2ExportRef string `json:"cur2_export_ref,omitempty"`
}

type ExecutionApproval struct {
	OperationID string `json:"operation_id"`
	Intent      string `json:"intent"`
	Confirmed   bool   `json:"confirmed"`
}

const (
	AWSBackfillSupportCaseOperationID        = "aws.billing.cur2.previous_month_backfill_support_case"
	ApprovalIntentRequestBackfillSupportCase = "request_backfill_support_case"
)

var (
	cur2ExportRefPattern      = regexp.MustCompile(`^cur2-[a-f0-9]+$`)
	awsAccountIDLikePattern   = regexp.MustCompile(`^\d{12}$`)
	awsAccessKeyIDLikePattern = regexp.MustCompile(`(?i)^(AKIA|ASIA)[A-Z0-9]{16}$`)
)

func DefaultExecutionOptions() ExecutionOptions {
	options, err := NormalizeExecutionOptions(ExecutionOptions{
		InterfaceMode:  InterfaceModeDirect,
		TimeoutSeconds: DefaultExecutionTimeoutSeconds,
	})
	if err != nil {
		panic(err)
	}
	return options
}

func NormalizeExecutionOptions(input ExecutionOptions) (ExecutionOptions, error) {
	if strings.TrimSpace(input.SchemaVersion) == "" {
		input.SchemaVersion = ExecutionOptionsSchemaVersion
	}
	if input.SchemaVersion != ExecutionOptionsSchemaVersion {
		return ExecutionOptions{}, fmt.Errorf("execution_options schema_version is unsupported")
	}
	if input.InterfaceMode == "" {
		input.InterfaceMode = InterfaceModeDirect
	}
	switch input.InterfaceMode {
	case InterfaceModeDirect, InterfaceModeGuided:
	default:
		return ExecutionOptions{}, fmt.Errorf("execution_options interface_mode is unsupported")
	}
	if input.TimeoutSeconds < 0 {
		return ExecutionOptions{}, fmt.Errorf("execution_options timeout_seconds must not be negative")
	}
	if input.TimeoutSeconds == 0 {
		input.TimeoutSeconds = DefaultExecutionTimeoutSeconds
	}

	selectors, err := normalizeExecutionSelectors(input.Selectors)
	if err != nil {
		return ExecutionOptions{}, err
	}
	input.Selectors = selectors
	approvals, err := normalizeExecutionApprovals(input.Approvals)
	if err != nil {
		return ExecutionOptions{}, err
	}
	input.Approvals = approvals
	return input, nil
}

func NormalizeExecutionOptionsForRequest(request Request, input ExecutionOptions) (ExecutionOptions, error) {
	options, err := NormalizeExecutionOptions(input)
	if err != nil {
		return ExecutionOptions{}, err
	}
	if options.Selectors != nil && options.Selectors.AWS != nil && !requestAllowsAWSBillingSelectors(request) {
		return ExecutionOptions{}, fmt.Errorf("AWS selector flags are supported only for matilda-prep rapid-assessment billing aws preflight or apply-prereqs")
	}
	if len(options.Approvals) > 0 && !requestAllowsAWSBackfillApproval(request) {
		return ExecutionOptions{}, fmt.Errorf("AWS backfill support case approval is supported only for matilda-prep rapid-assessment billing aws apply-prereqs")
	}
	for _, approval := range options.Approvals {
		if approval.OperationID != AWSBackfillSupportCaseOperationID ||
			approval.Intent != ApprovalIntentRequestBackfillSupportCase {
			return ExecutionOptions{}, fmt.Errorf("approval is not recognized for this implementation slice")
		}
		if !approval.Confirmed {
			return ExecutionOptions{}, fmt.Errorf("approval must be explicitly confirmed")
		}
	}
	return options, nil
}

func HasAWSBackfillSupportCaseApproval(options ExecutionOptions) bool {
	for _, approval := range options.Approvals {
		if approval.OperationID == AWSBackfillSupportCaseOperationID &&
			approval.Intent == ApprovalIntentRequestBackfillSupportCase &&
			approval.Confirmed {
			return true
		}
	}
	return false
}

func normalizeExecutionSelectors(input *ExecutionSelectors) (*ExecutionSelectors, error) {
	if input == nil {
		return nil, nil
	}

	normalized := &ExecutionSelectors{}
	awsSelectors, err := normalizeAWSExecutionSelectors(input.AWS)
	if err != nil {
		return nil, err
	}
	if awsSelectors != nil {
		normalized.AWS = awsSelectors
	}
	if normalized.AWS == nil {
		return nil, nil
	}
	return normalized, nil
}

func normalizeExecutionApprovals(input []ExecutionApproval) ([]ExecutionApproval, error) {
	if len(input) == 0 {
		return nil, nil
	}
	normalized := make([]ExecutionApproval, 0, len(input))
	for index, approval := range input {
		approval.OperationID = strings.TrimSpace(approval.OperationID)
		approval.Intent = strings.TrimSpace(approval.Intent)
		if approval.OperationID == "" {
			return nil, fmt.Errorf("approval %d operation_id is required", index)
		}
		if approval.Intent == "" {
			return nil, fmt.Errorf("approval %d intent is required", index)
		}
		if err := ensureSafeText("execution_options approval", approval.OperationID, approval.Intent); err != nil {
			return nil, fmt.Errorf("approval %d: %w", index, err)
		}
		if approval.OperationID != AWSBackfillSupportCaseOperationID {
			return nil, fmt.Errorf("approval %d operation_id is unsupported", index)
		}
		if approval.Intent != ApprovalIntentRequestBackfillSupportCase {
			return nil, fmt.Errorf("approval %d intent is unsupported", index)
		}
		normalized = append(normalized, approval)
	}
	return normalized, nil
}

func normalizeAWSExecutionSelectors(input *AWSExecutionSelectors) (*AWSExecutionSelectors, error) {
	if input == nil {
		return nil, nil
	}

	normalized := &AWSExecutionSelectors{
		Profile:       strings.TrimSpace(input.Profile),
		Region:        strings.TrimSpace(input.Region),
		CUR2ExportRef: strings.TrimSpace(input.CUR2ExportRef),
	}
	if normalized.Profile != "" {
		if err := ensureSafeText("execution_options aws profile", normalized.Profile); err != nil {
			return nil, fmt.Errorf("profile: %w", err)
		}
		if pathLikeSelectorValue(normalized.Profile) {
			return nil, fmt.Errorf("profile: execution_options aws profile must not be a local path")
		}
		if sensitiveIdentifierLikeSelectorValue(normalized.Profile) {
			return nil, fmt.Errorf("profile: execution_options aws profile must not look like sensitive cloud identifier material")
		}
	}
	if normalized.Region != "" {
		if err := ensureSafeText("execution_options aws region", normalized.Region); err != nil {
			return nil, fmt.Errorf("region: %w", err)
		}
		if pathLikeSelectorValue(normalized.Region) {
			return nil, fmt.Errorf("region: execution_options aws region must not be a local path")
		}
		if sensitiveIdentifierLikeSelectorValue(normalized.Region) {
			return nil, fmt.Errorf("region: execution_options aws region must not look like sensitive cloud identifier material")
		}
	}
	if normalized.CUR2ExportRef != "" {
		if err := ensureSafeText("execution_options aws cur2_export_ref", normalized.CUR2ExportRef); err != nil {
			return nil, fmt.Errorf("cur2_export_ref: %w", err)
		}
		if !validCUR2ExportRef(normalized.CUR2ExportRef) {
			return nil, fmt.Errorf("cur2_export_ref must use format cur2- plus 16, 24, or 32 lowercase hexadecimal characters")
		}
	}
	if normalized.Profile == "" && normalized.Region == "" && normalized.CUR2ExportRef == "" {
		return nil, nil
	}
	return normalized, nil
}

func validCUR2ExportRef(value string) bool {
	if !cur2ExportRefPattern.MatchString(value) {
		return false
	}
	hexLength := len(value) - len("cur2-")
	return hexLength == 16 || hexLength == 24 || hexLength == 32
}

func pathLikeSelectorValue(value string) bool {
	return strings.ContainsAny(value, `/\`)
}

func sensitiveIdentifierLikeSelectorValue(value string) bool {
	return awsAccountIDLikePattern.MatchString(value) || awsAccessKeyIDLikePattern.MatchString(value)
}

func requestAllowsAWSBillingSelectors(request Request) bool {
	return request.Goal == "rapid-assessment" &&
		request.CollectionPath == "billing" &&
		request.Provider == "aws" &&
		(request.Action == "preflight" || request.Action == "apply-prereqs")
}

func requestAllowsAWSBackfillApproval(request Request) bool {
	return request.Goal == "rapid-assessment" &&
		request.CollectionPath == "billing" &&
		request.Provider == "aws" &&
		request.Action == "apply-prereqs"
}
