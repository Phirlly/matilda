package awsclient

import (
	"errors"
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingcur2setup"
	smithy "github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func classifyDataExportsError(err error, fallback string) error {
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		switch {
		case isAccessDenied(code):
			return billingcur2setup.NewProviderError("aws_data_exports_access_denied", "AWS Data Exports access was denied.")
		case isThrottle(code):
			return billingcur2setup.NewProviderError("aws_data_exports_throttled", "AWS Data Exports request was throttled.")
		case isQuota(code):
			return billingcur2setup.NewProviderError("aws_cur2_export_quota_full", "AWS CUR 2.0 Data Exports quota is full.")
		case isValidation(code):
			return billingcur2setup.NewProviderError(fallback, "AWS Data Exports rejected the create-export request.")
		}
	}
	return billingcur2setup.NewProviderError(fallback, "AWS Data Exports request failed.")
}

func classifyS3Error(err error, fallback string) error {
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		switch {
		case strings.EqualFold(code, "BucketAlreadyOwnedByYou"):
			return billingcur2setup.NewProviderError("aws_s3_bucket_already_owned_by_caller", "AWS S3 bucket is already owned by the caller.")
		case strings.EqualFold(code, "BucketAlreadyExists"):
			return billingcur2setup.NewProviderError("aws_s3_bucket_already_exists", "AWS S3 bucket name is unavailable.")
		case strings.EqualFold(code, "MethodNotAllowed"):
			return billingcur2setup.NewProviderError("aws_s3_bucket_owner_mismatch", "AWS S3 bucket owner could not be proved with the expected owner condition.")
		case isAccessDenied(code), isValidation(code), isNotFound(code), isThrottle(code):
			return billingcur2setup.NewProviderError(fallback, "AWS S3 request failed.")
		}
	}
	return billingcur2setup.NewProviderError(fallback, "AWS S3 request failed.")
}

func apiErrorCode(err error) string {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode()
	}
	return ""
}

func classifyOrganizationsError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		switch {
		case strings.EqualFold(code, "AWSOrganizationsNotInUseException"):
			return billingcur2setup.NewProviderError("aws_organizations_not_in_use", "AWS Organizations is not in use for this account.")
		case isAccessDenied(code):
			return billingcur2setup.NewProviderError("aws_organizations_access_denied", "AWS Organizations access was denied.")
		case isThrottle(code):
			return billingcur2setup.NewProviderError("aws_organizations_unavailable", "AWS Organizations request was throttled.")
		}
	}
	return billingcur2setup.NewProviderError("aws_organizations_unavailable", "AWS Organizations coverage could not be verified.")
}

func statusCode(err error) int {
	var responseErr *smithyhttp.ResponseError
	if errors.As(err, &responseErr) && responseErr.HTTPResponse() != nil && responseErr.HTTPResponse().Response != nil {
		return responseErr.HTTPStatusCode()
	}
	return 0
}

func isNoBucketPolicy(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && containsAnyFold(apiErr.ErrorCode(), "NoSuchBucketPolicy")
}

func isAccessDenied(code string) bool {
	return containsAnyFold(code, "AccessDenied", "Unauthorized", "Forbidden", "UnrecognizedClient")
}

func isThrottle(code string) bool {
	return containsAnyFold(code, "Throttl", "TooManyRequests")
}

func isValidation(code string) bool {
	return containsAnyFold(code, "Validation", "Invalid")
}

func isNotFound(code string) bool {
	return containsAnyFold(code, "NotFound", "NoSuchBucket")
}

func isNoSuchBucket(code string) bool {
	return strings.EqualFold(code, "NoSuchBucket")
}

func isQuota(code string) bool {
	return containsAnyFold(code, "LimitExceeded", "Quota")
}

func containsAnyFold(value string, parts ...string) bool {
	lower := strings.ToLower(value)
	for _, part := range parts {
		if strings.Contains(lower, strings.ToLower(part)) {
			return true
		}
	}
	return false
}
