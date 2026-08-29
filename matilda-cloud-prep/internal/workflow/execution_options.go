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
	SchemaVersion       string              `json:"schema_version"`
	InterfaceMode       InterfaceMode       `json:"interface_mode"`
	TimeoutSeconds      int                 `json:"timeout_seconds"`
	Selectors           *ExecutionSelectors `json:"selectors,omitempty"`
	AWSBillingOperation AWSBillingOperation `json:"aws_billing_operation,omitempty"`
	Approvals           []ExecutionApproval `json:"approvals,omitempty"`
}

type ExecutionSelectors struct {
	AWS *AWSExecutionSelectors `json:"aws,omitempty"`
}

type AWSExecutionSelectors struct {
	Profile       string `json:"profile,omitempty"`
	Region        string `json:"region,omitempty"`
	CUR2ExportRef string `json:"cur2_export_ref,omitempty"`
}

type AWSBillingOperation string

const (
	AWSBillingOperationRequestBackfill  AWSBillingOperation = "request_backfill"
	AWSBillingOperationCreateCUR2Export AWSBillingOperation = "create_cur2_export"
	AWSBillingOperationConflict         AWSBillingOperation = "conflict"
)

type ExecutionApproval struct {
	OperationID string `json:"operation_id"`
	Intent      string `json:"intent,omitempty"`
	PlanID      string `json:"plan_id,omitempty"`
	Confirmed   bool   `json:"confirmed"`
}

const (
	AWSBackfillSupportCaseOperationID        = "aws.billing.cur2.previous_month_backfill_support_case"
	ApprovalIntentRequestBackfillSupportCase = "request_backfill_support_case"
	AWSCUR2CreateBucketOperationID           = "aws.billing.cur2.bucket.create"
	AWSCUR2MergeBucketPolicyOperationID      = "aws.billing.cur2.bucket_policy.merge_data_exports_delivery"
	AWSCUR2CreateExportOperationID           = "aws.billing.cur2.export.create"
)

var (
	generatedCUR2ExportRefPattern = regexp.MustCompile(`^cur2-[a-p]+$`)
	planIDPattern                 = regexp.MustCompile(`^plan_[a-p]{16}$`)
	awsAccountIDLikePattern       = regexp.MustCompile(`^\d{12}$`)
	awsAccessKeyIDLikePattern     = regexp.MustCompile(`(?i)^(AKIA|ASIA)[A-Z0-9]{16}$`)
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
	operation, err := normalizeAWSBillingOperation(input.AWSBillingOperation)
	if err != nil {
		return ExecutionOptions{}, err
	}
	input.AWSBillingOperation = operation

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
		return ExecutionOptions{}, fmt.Errorf("AWS selector flags are supported only for matilda-prep rapid-assessment billing aws preflight, apply-prereqs, or package")
	}
	if options.AWSBillingOperation != "" && !requestAllowsAWSBillingApplyPrereqs(request) {
		return ExecutionOptions{}, fmt.Errorf("AWS billing operation flags are supported only for matilda-prep rapid-assessment billing aws apply-prereqs")
	}
	if options.AWSBillingOperation == AWSBillingOperationConflict {
		return ExecutionOptions{}, fmt.Errorf("aws_billing_prereqs_operation_conflict")
	}
	if len(options.Approvals) > 0 && !requestAllowsAWSBillingApplyPrereqs(request) {
		return ExecutionOptions{}, fmt.Errorf("AWS billing approval is supported only for matilda-prep rapid-assessment billing aws apply-prereqs")
	}
	for _, approval := range options.Approvals {
		if !approval.Confirmed {
			return ExecutionOptions{}, fmt.Errorf("approval must be explicitly confirmed")
		}
		switch approval.OperationID {
		case AWSBackfillSupportCaseOperationID:
			if options.AWSBillingOperation != AWSBillingOperationRequestBackfill {
				return ExecutionOptions{}, fmt.Errorf("approval requires a matching AWS billing operation")
			}
			if approval.Intent != ApprovalIntentRequestBackfillSupportCase {
				return ExecutionOptions{}, fmt.Errorf("approval is not recognized for this implementation slice")
			}
		case AWSCUR2CreateBucketOperationID, AWSCUR2MergeBucketPolicyOperationID, AWSCUR2CreateExportOperationID:
			if options.AWSBillingOperation != AWSBillingOperationCreateCUR2Export {
				return ExecutionOptions{}, fmt.Errorf("plan-bound approval requires a matching AWS billing operation")
			}
			if approval.Intent != "" {
				return ExecutionOptions{}, fmt.Errorf("approval intent is unsupported for create-cur2-export")
			}
		default:
			return ExecutionOptions{}, fmt.Errorf("approval is not recognized for this implementation slice")
		}
	}
	return options, nil
}

func HasAWSBillingOperation(options ExecutionOptions, operation AWSBillingOperation) bool {
	return options.AWSBillingOperation == operation
}

func HasApprovedPlanStep(options ExecutionOptions, planID string, operationID string) bool {
	for _, approval := range options.Approvals {
		if approval.OperationID == operationID &&
			approval.PlanID == planID &&
			approval.Confirmed {
			return true
		}
	}
	return false
}

func normalizeAWSBillingOperation(input AWSBillingOperation) (AWSBillingOperation, error) {
	operation := AWSBillingOperation(strings.TrimSpace(string(input)))
	switch operation {
	case "", AWSBillingOperationRequestBackfill, AWSBillingOperationCreateCUR2Export, AWSBillingOperationConflict:
		if operation != "" {
			if err := ensureSafeText("execution_options aws billing operation", string(operation)); err != nil {
				return "", err
			}
		}
		return operation, nil
	default:
		return "", fmt.Errorf("execution_options aws_billing_operation is unsupported")
	}
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
		approval.PlanID = strings.TrimSpace(approval.PlanID)
		if err := ensureSafeText("execution_options approval", approval.OperationID, approval.Intent, approval.PlanID); err != nil {
			return nil, fmt.Errorf("approval %d: %w", index, err)
		}
		switch approval.OperationID {
		case AWSBackfillSupportCaseOperationID:
			if approval.Intent == "" {
				return nil, fmt.Errorf("approval %d intent is required", index)
			}
			if approval.Intent != ApprovalIntentRequestBackfillSupportCase {
				return nil, fmt.Errorf("approval %d intent is unsupported", index)
			}
			if approval.PlanID == "" {
				return nil, fmt.Errorf("approval %d plan_id is required", index)
			}
			if !validPlanID(approval.PlanID) {
				return nil, fmt.Errorf("approval %d plan_id must use format plan_ plus 16 lowercase account-id-safe characters", index)
			}
		case AWSCUR2CreateBucketOperationID, AWSCUR2MergeBucketPolicyOperationID, AWSCUR2CreateExportOperationID:
			if approval.PlanID == "" {
				return nil, fmt.Errorf("approval %d plan_id is required", index)
			}
			if !validPlanID(approval.PlanID) {
				return nil, fmt.Errorf("approval %d plan_id must use format plan_ plus 16 lowercase account-id-safe characters", index)
			}
		default:
			return nil, fmt.Errorf("approval %d operation_id is unsupported", index)
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
			return nil, fmt.Errorf("cur2_export_ref must use format cur2- plus 16, 24, or 32 lowercase generated reference characters")
		}
	}
	if normalized.Profile == "" && normalized.Region == "" && normalized.CUR2ExportRef == "" {
		return nil, nil
	}
	return normalized, nil
}

func validCUR2ExportRef(value string) bool {
	if !generatedCUR2ExportRefPattern.MatchString(value) {
		return false
	}
	refLength := len(value) - len("cur2-")
	return refLength == 16 || refLength == 24 || refLength == 32
}

func validPlanID(value string) bool {
	return planIDPattern.MatchString(value)
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
		(request.Action == "preflight" || request.Action == "apply-prereqs" || request.Action == "package")
}

func requestAllowsAWSBillingApplyPrereqs(request Request) bool {
	return request.Goal == "rapid-assessment" &&
		request.CollectionPath == "billing" &&
		request.Provider == "aws" &&
		request.Action == "apply-prereqs"
}
