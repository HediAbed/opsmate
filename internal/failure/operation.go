package failure

type Operation string

const (
	OperationAnalyze   Operation = "analyze"
	OperationBuild     Operation = "build"
	OperationChat      Operation = "chat"
	OperationCollect   Operation = "collect"
	OperationConfigure Operation = "configure"
	OperationConnect   Operation = "connect"
	OperationCreate    Operation = "create"
	OperationDecode    Operation = "decode"
	OperationDelete    Operation = "delete"
	OperationEncode    Operation = "encode"
	OperationGet       Operation = "get"
	OperationList      Operation = "list"
	OperationLoad      Operation = "load"
	OperationObserve   Operation = "observe"
	OperationRead      Operation = "read"
	OperationResolve   Operation = "resolve"
	OperationSend      Operation = "send"
	OperationSet       Operation = "set"
	OperationStart     Operation = "start"
	OperationStop      Operation = "stop"
	OperationStream    Operation = "stream"
	OperationUpdate    Operation = "update"
	OperationValidate  Operation = "validate"
)
