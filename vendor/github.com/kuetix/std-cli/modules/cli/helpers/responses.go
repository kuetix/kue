package helpers

import (
	"fmt"
	"os"
)

func RespondErrors(errorMessages []string, statusCode int) {
	for _, message := range errorMessages {
		println(message)
	}
	if statusCode >= 200 {
		statusCode -= 200
	}
	os.Exit(statusCode)
}

func RespondSuccess(response any) {
	println(fmt.Sprintf("%v", response))
	os.Exit(0)
}
