package server

const (
	ResponseRejected             = "RESPONSE|REJECTED|"
	ResponseAccepted             = "RESPONSE|ACCEPTED|"
	ResponseTransactionProcessed = ResponseAccepted + "Transaction processed"
	ResponseInvalidRequest       = ResponseRejected + "Invalid request"
	ResponseInvalidAmount        = ResponseRejected + "Invalid amount"
	ResponseCancelled            = ResponseRejected + "Cancelled"
)
