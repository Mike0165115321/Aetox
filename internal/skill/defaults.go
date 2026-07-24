package skill

type RegistryOptions struct {
	SandboxRoot string
}

func NewDefaultRegistry(opts RegistryOptions) *Registry {
	registry := NewRegistry()
	RegisterDefaults(registry, opts)
	return registry
}

func RegisterDefaults(registry *Registry, opts RegistryOptions) {
	if registry == nil {
		return
	}
	defaults := []Skill{
		&helpSkill{registry: registry},
		&echoSkill{},
		&timeSkill{},
		&listSkill{root: opts.SandboxRoot},
		&readSkill{root: opts.SandboxRoot},
		&githubRepoSummarySkill{},
		&gitSkill{root: opts.SandboxRoot},
		&fsSkill{root: opts.SandboxRoot},
		&shellSkill{root: opts.SandboxRoot},
		&writeSkill{root: opts.SandboxRoot},
		&editSkill{root: opts.SandboxRoot},
		&grepSkill{root: opts.SandboxRoot},
		&deleteSkill{root: opts.SandboxRoot},
		&pluginInstallSkill{},
		&imageOCRSkill{root: opts.SandboxRoot},
		&webFetchSkill{},
		&webSearchSkill{},
		&githubSearchSkill{},
		&githubReadFileSkill{},
		&githubListFilesSkill{},
	}
	for _, s := range defaults {
		if err := registry.Register(s, SourceBuiltin); err != nil {
			// Two built-ins sharing a name is a programmer error, not a runtime condition.
			panic(err)
		}
	}
}
