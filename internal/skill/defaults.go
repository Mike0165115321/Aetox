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
	registry.Register(&helpSkill{registry: registry})
	registry.Register(&echoSkill{})
	registry.Register(&timeSkill{})
	registry.Register(&listSkill{root: opts.SandboxRoot})
	registry.Register(&gitSkill{root: opts.SandboxRoot})
	registry.Register(&fsSkill{root: opts.SandboxRoot})
	registry.Register(&shellSkill{root: opts.SandboxRoot})
	registry.Register(&writeSkill{root: opts.SandboxRoot})
}
