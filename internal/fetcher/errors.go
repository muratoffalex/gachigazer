package fetcher

import "errors"

var (
	ErrNotHandle     = errors.New("not handling")
	ErrInvalidURL    = errors.New("invalid URL")
	ErrCannotBeEmpty = errors.New("URL cannot be empty")
)
