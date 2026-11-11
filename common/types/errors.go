package types

func (err AccessDeniedError) Error() string {
	return err.Message
}

func (err BadRequestError) Error() string {
	return err.Message
}

func (err EntityNotExistsError) Error() string {
	return err.Message
}

func (err InternalServiceError) Error() string {
	return err.Message
}

func (err ServiceBusyError) Error() string {
	return err.Message
}
