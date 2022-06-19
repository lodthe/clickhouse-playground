package qrunner

import "github.com/pkg/errors"

var ErrNoAvailableRunners = errors.New("no available runners, try again later")
