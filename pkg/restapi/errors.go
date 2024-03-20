package restapi

import "github.com/pkg/errors"

var ErrUnknownDatabase = errors.New("unknown database")
var ErrMissingRunSettings = errors.New("missing run settings")
