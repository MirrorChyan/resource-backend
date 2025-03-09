package errs

type Error struct {
	StatusCode int
	Message    string
	Data       any
}

func (e *Error) Error() string {
	return e.Message
}
