package constant

const (
	Zero uint = iota
	One
	Two
	Three
	Four
	Five
)

const (
	MinPageNum  = 1
	MinPageSize = 1
	PageSize    = 10
	MaxPageSize = 5000
)

// mode
const (
	DEV   = "dev"
	UAT   = "uat"
	STAGE = "stage"
	PROD  = "prod"
)
