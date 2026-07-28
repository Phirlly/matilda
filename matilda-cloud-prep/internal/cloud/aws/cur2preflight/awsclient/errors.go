package awsclient

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	smithy "github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func providerError(code string) error {
	return cur2preflight.NewProviderError(code, messageForCode(code))
}

func messageForCode(code string) string {
	switch code {
	case "aws_config_missing_region":
		return "AWS Region is not configured."
	case "aws_config_missing_credentials":
		return "AWS credentials are not available."
	case "aws_config_timeout":
		return "AWS SDK configuration timed out."
	case "aws_config_cancelled":
		return "AWS SDK configuration was cancelled."
	case "aws_config_profile_shadowed":
		return "AWS profile selection is blocked because credential environment variables would take precedence."
	case "aws_auth_failed":
		return "AWS caller identity could not be verified."
	case "aws_data_exports_access_denied":
		return "AWS Data Exports access is not available."
	case "aws_data_exports_throttled":
		return "AWS Data Exports request was throttled."
	case "aws_data_exports_transient":
		return "AWS Data Exports request failed with a transient provider error."
	case "aws_cur2_table_unavailable":
		return "AWS CUR 2.0 table metadata is not available."
	case "aws_cur2_export_invalid_shape":
		return "AWS CUR 2.0 export metadata is incomplete or invalid."
	case "aws_s3_bucket_policy_inaccessible":
		return "AWS S3 bucket policy could not be inspected."
	case "aws_s3_bucket_inaccessible":
		return "AWS S3 bucket could not be inspected."
	default:
		return "AWS provider call failed."
	}
}

func classifyConfigurationError(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return providerError("aws_config_timeout")
	case errors.Is(err, context.Canceled):
		return providerError("aws_config_cancelled")
	default:
		return providerError("aws_config_missing_credentials")
	}
}

func classifyDataExportsError(err error, fallback string) error {
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		switch {
		case isAccessDenied(code):
			return providerError("aws_data_exports_access_denied")
		case isThrottle(code):
			return providerError("aws_data_exports_throttled")
		case isTransient(code) || apiErr.ErrorFault() == smithy.FaultServer:
			return providerError("aws_data_exports_transient")
		case isNotFound(code), isValidation(code):
			return providerError(fallback)
		}
	}
	return providerError(fallback)
}

func classifyExecutionDetailError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && isValidation(apiErr.ErrorCode()) {
		return providerError("aws_cur2_export_invalid_shape")
	}
	return classifyDataExportsError(err, "aws_data_exports_access_denied")
}

func classifySTSSError(err error) error {
	if err == nil {
		return nil
	}
	return providerError("aws_auth_failed")
}

func classifyS3Error(err error, fallback string) error {
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		switch {
		case isThrottle(code), isTransient(code), isAccessDenied(code), isNotFound(code), isValidation(code), strings.EqualFold(code, "MethodNotAllowed"):
			return providerError(fallback)
		}
	}
	return providerError(fallback)
}

func headBucketStatus(err error) (cur2preflight.BucketAccess, bool) {
	var responseErr *smithyhttp.ResponseError
	if !errors.As(err, &responseErr) || responseErr.HTTPResponse() == nil || responseErr.HTTPResponse().Response == nil {
		return cur2preflight.BucketAccess{}, false
	}
	response := responseErr.HTTPResponse().Response
	return cur2preflight.BucketAccess{
		Accessible: false,
		StatusCode: response.StatusCode,
		Region:     response.Header.Get("X-Amz-Bucket-Region"),
	}, true
}

func statusCode(err error) int {
	var responseErr *smithyhttp.ResponseError
	if errors.As(err, &responseErr) && responseErr.HTTPResponse() != nil && responseErr.HTTPResponse().Response != nil {
		return responseErr.HTTPStatusCode()
	}
	return 0
}

func isAccessDenied(code string) bool {
	return containsAnyFold(code, "AccessDenied", "Unauthorized", "Forbidden", "UnrecognizedClient")
}

func isThrottle(code string) bool {
	return containsAnyFold(code, "Throttl", "TooManyRequests")
}

func isTransient(code string) bool {
	return containsAnyFold(code, "InternalServer", "RequestTimeout", "Timeout", "ServiceUnavailable")
}

func isNotFound(code string) bool {
	return containsAnyFold(code, "NotFound", "NoSuchBucket", "ResourceNotFound")
}

func isValidation(code string) bool {
	return containsAnyFold(code, "Validation", "Invalid")
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

func isAmbiguousHeadBucketStatus(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusMethodNotAllowed:
		return true
	default:
		return false
	}
}
