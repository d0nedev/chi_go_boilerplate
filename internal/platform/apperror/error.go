package apperror

import (
	"fmt"
	"net/http"
	"runtime"
)

type Error struct {
	Status  int
	Code    string
	Message string
	Err     error
	stack   []uintptr
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Err
}

func New(status int, code string, message string) *Error {
	return &Error{
		Status:  status,
		Message: message,
		Code:    code,
		stack:   captureStack(1),
	}
}

func Wrap(status int, code string, message string, err error) *Error {
	return &Error{
		Status:  status,
		Code:    code,
		Message: message,
		Err:     err,
		stack:   captureStack(1),
	}
}

func NotFound(code string, message string) *Error {
	return New(http.StatusNotFound, code, message)
}

// Internal wraps an unexpected failure; message is shown to clients, err only in logs and traces.
func Internal(code string, message string, err error) *Error {
	appErr := Wrap(http.StatusInternalServerError, code, message, err)
	appErr.stack = captureStack(1)
	return appErr
}

func Validation(message string) *Error {
	return New(
		http.StatusBadRequest,
		CodeValidation,
		message,
	)
}

func (e *Error) StackTrace() []string {
	if len(e.stack) == 0 {
		return nil
	}

	frames := runtime.CallersFrames(e.stack)

	result := make([]string, 0, len(e.stack))

	for {
		frame, more := frames.Next()

		result = append(
			result,
			fmt.Sprintf(
				"%s:%d",
				frame.File,
				frame.Line,
			),
		)

		if !more {
			break
		}
	}
	return result
}

func captureStack(skip int) []uintptr {
	const maxDepth = 32

	pcs := make([]uintptr, maxDepth)

	n := runtime.Callers(
		skip+2,
		pcs,
	)

	return pcs[:n]
}
