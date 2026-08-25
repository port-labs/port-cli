package commands

import "fmt"

const (
	defaultMaxErrors = 5
	hideAllErrors    = -1
)

func validateMaxErrorsFlag(maxErrors int) error {
	if maxErrors < hideAllErrors {
		return fmt.Errorf("--max-errors must be -1, 0, or greater")
	}
	return nil
}

func errorLimit(totalErrors, maxErrors int) int {
	if totalErrors <= 0 || maxErrors == hideAllErrors {
		return 0
	}
	if maxErrors == 0 || maxErrors > totalErrors {
		return totalErrors
	}
	return maxErrors
}

func shouldPrintErrors(totalErrors, maxErrors int) bool {
	return errorLimit(totalErrors, maxErrors) > 0
}
