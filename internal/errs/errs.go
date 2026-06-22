package errs

const (
	AuthMissingCredentials     = "AUTH_MISSING_CREDENTIALS" // #nosec G101 -- stable error subtype, not a credential.
	AuthInvalidClient          = "AUTH_INVALID_CLIENT"
	AuthTokenRefreshFailed     = "AUTH_TOKEN_REFRESH_FAILED" // #nosec G101 -- stable error subtype, not a credential.
	ChannelMissing             = "CHANNEL_MISSING"
	ChannelAliasNotFound       = "CHANNEL_ALIAS_NOT_FOUND"
	ChannelInvalid             = "CHANNEL_INVALID"
	ChannelBatchFailed         = "CHANNEL_BATCH_FAILED"
	ValidationRequiredFlag     = "VALIDATION_REQUIRED_FLAG"
	ValidationBadParam         = "VALIDATION_BAD_PARAM"
	InputPathUnsafe            = "INPUT_PATH_UNSAFE"
	RateLimitExceeded          = "RATE_LIMIT_EXCEEDED"
	HTTP5xx                    = "HTTP_5XX"
	NetworkFailed              = "NETWORK_FAILED"
	ConfirmationRequired       = "CONFIRMATION_REQUIRED"
	WriteConfirmationMismatch  = "WRITE_CONFIRMATION_MISMATCH"
	WriteConfirmationExpired   = "WRITE_CONFIRMATION_EXPIRED"
	WriteConfirmationReplay    = "WRITE_CONFIRMATION_REPLAY"
	WriteMultiChannelForbidden = "WRITE_MULTI_CHANNEL_FORBIDDEN"
	WriteNotAllowlisted        = "WRITE_NOT_ALLOWLISTED"
	APIBusinessError           = "API_BUSINESS_ERROR"
)

type Error struct {
	KindValue    string
	SubtypeValue string
	Message      string
	HintValue    string
}

func New(kind, subtype, message, hint string) error {
	return &Error{KindValue: kind, SubtypeValue: subtype, Message: message, HintValue: hint}
}

func (e *Error) Error() string   { return e.Message }
func (e *Error) Kind() string    { return e.KindValue }
func (e *Error) Subtype() string { return e.SubtypeValue }
func (e *Error) Hint() string    { return e.HintValue }

func Hint(subtype string) string {
	switch subtype {
	case AuthMissingCredentials:
		return "configure credentials with cbi config init --client-secret-stdin, --client-secret-from-env, --client-secret-file, or CAPTAINBI_ACCESS_TOKEN"
	case AuthInvalidClient:
		return "verify CaptainBI APPID/client_secret and rerun cbi config init; token requests include scope=all automatically"
	case AuthTokenRefreshFailed:
		return "run cbi auth status --machine, then refresh credentials with cbi auth token or cbi config init"
	case ChannelMissing:
		return "run cbi +shops, then pass --channel <alias> or configure cbi config channels add <alias> <open_channel_id>"
	case ChannelAliasNotFound:
		return "run cbi config channels list or cbi +shops, then use a configured alias; use --open-channel-id for a raw ID"
	case ChannelInvalid:
		return "verify the channel alias or OpenChannelId with cbi +shops, then update cbi config channels"
	case ChannelBatchFailed:
		return "inspect data.channels, fix each failed channel, then retry only the affected aliases"
	case ValidationRequiredFlag:
		return "run the command with --help and pass the required flag shown in Examples"
	case ValidationBadParam:
		return "fix the parameter value according to --help or cbi schema <domain.command>"
	case InputPathUnsafe:
		return "use a relative file inside the current working directory, or pass absolute-path content through stdin"
	case RateLimitExceeded:
		return "wait retry_after_ms when present, reduce concurrency, or lower --rate-limit"
	case HTTP5xx:
		return "retry later; CaptainBI returned a server error"
	case NetworkFailed:
		return "retry later and check network or proxy settings"
	case ConfirmationRequired:
		return "use --dry-run to preview, then pass its --confirm-request hash only after explicit user approval"
	case WriteConfirmationMismatch, WriteConfirmationExpired, WriteConfirmationReplay:
		return "rerun --dry-run, ask the user to approve the exact preview, then pass its current --confirm-request hash"
	case WriteMultiChannelForbidden:
		return "write to one configured channel alias at a time and approve each payload separately"
	case WriteNotAllowlisted:
		return "review the command risk, then add its domain.command reference with cbi config write-allowlist add"
	case APIBusinessError:
		return "read api_code/api_msg, fix the request parameters or channel, then retry"
	default:
		return ""
	}
}
