package spec

const (
	ParseErrorCode          int = -32700
	InvalidParamsErrorCode  int = -32602
	MethodNotFoundErrorCode int = -32601
	InvalidRequestErrorCode int = -32600
	ServerErrorCode         int = -32000

	// ClientTimeBudgetErrorCode answers a call refused once its client has
	// spent its budget of indexer time. It is a server error of this
	// indexer's own, in the -32000 to -32099 range JSON-RPC 2.0 reserves for
	// them beside ServerErrorCode.
	ClientTimeBudgetErrorCode int = -32005
)
