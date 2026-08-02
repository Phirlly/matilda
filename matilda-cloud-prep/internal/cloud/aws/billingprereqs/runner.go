package billingprereqs

import (
	"context"
	"reflect"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

type RunnerConfig struct {
	BackfillRunner workflow.CapabilityRunner
	SetupRunner    workflow.CapabilityRunner
}

type Runner struct {
	backfillRunner workflow.CapabilityRunner
	setupRunner    workflow.CapabilityRunner
}

func NewRunner(config RunnerConfig) Runner {
	return Runner{
		backfillRunner: config.BackfillRunner,
		setupRunner:    config.SetupRunner,
	}
}

func (runner Runner) Run(ctx context.Context, request workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
	switch options.AWSBillingOperation {
	case "":
		return operationRequiredReport(request)
	case workflow.AWSBillingOperationRequestBackfill:
		if isNilRunner(runner.backfillRunner) {
			return blockedReport(request, "aws_billing_prereqs_backfill_runner_unavailable", "AWS billing backfill prerequisites runner is not configured.")
		}
		return runner.backfillRunner.Run(ctx, request, options)
	case workflow.AWSBillingOperationCreateCUR2Export:
		if isNilRunner(runner.setupRunner) {
			return blockedReport(request, "aws_billing_prereqs_setup_runner_unavailable", "AWS CUR 2.0 create-export prerequisites runner is not configured.")
		}
		return runner.setupRunner.Run(ctx, request, options)
	case workflow.AWSBillingOperationConflict:
		return blockedReport(request, "aws_billing_prereqs_operation_conflict", "Only one AWS billing prerequisites operation can be applied at a time.")
	default:
		return blockedReport(request, "aws_billing_prereqs_operation_unsupported", "AWS billing prerequisites operation is not supported by this implementation.")
	}
}

func isNilRunner(runner workflow.CapabilityRunner) bool {
	if runner == nil {
		return true
	}
	value := reflect.ValueOf(runner)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

var _ workflow.CapabilityRunner = Runner{}
