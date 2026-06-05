package config

import "os"

type Config struct {
	SandboxRoot        string
	AutoApprove        bool
	MaxRetries         int
	ApprovalTimeoutSec int
	MaxOutputFiles     int
}

type ConfigOptions struct {
	RootPath        string
	AutoApprove     bool
	MaxRetries      int
	ApprovalTimeout int
}

func Load(opt ConfigOptions) Config {
	root := opt.RootPath
	if root == "" {
		root, _ = os.Getwd()
	}

	maxRetries := opt.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 2
	}

	timeout := opt.ApprovalTimeout
	if timeout <= 0 {
		timeout = 60
	}

	return Config{
		SandboxRoot:        root,
		AutoApprove:        opt.AutoApprove,
		MaxRetries:         maxRetries,
		ApprovalTimeoutSec: timeout,
		MaxOutputFiles:     2000,
	}
}

