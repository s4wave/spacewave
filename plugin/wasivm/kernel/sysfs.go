package kernel

// newSysFS creates the root of a sysfs: the CPU files that describe the
// kernel's one CPU, numbered zero, as programs such as lscpu and nproc read
// them. The kernel has no devices, buses or drivers to show.
func (k *Kernel) newSysFS() *VNode {
	// Add the CPU directory.
	v := k.vfs
	root := v.newDir(0o755)
	system := v.mkDir(v.mkDir(root, "devices"), "system")
	cpu := v.mkDir(system, "cpu")

	// Each CPU list contains CPU zero alone.
	for _, name := range []string{"possible", "present", "online"} {
		v.mkGenFile(cpu, name, func() []byte { return []byte("0\n") })
	}
	v.mkGenFile(cpu, "kernel_max", func() []byte { return []byte("0\n") })

	// CPU zero is the one thread of the one core of the one package: each
	// topology id is zero, each sibling list contains it alone, and each
	// sibling mask holds its bit.
	topology := v.mkDir(v.mkDir(cpu, "cpu0"), "topology")
	files := map[string]string{
		"core_id": "0", "die_id": "0", "cluster_id": "0", "physical_package_id": "0",
		"thread_siblings_list": "0", "core_cpus_list": "0", "core_siblings_list": "0",
		"cluster_cpus_list": "0", "die_cpus_list": "0", "package_cpus_list": "0",
		"thread_siblings": "1", "core_cpus": "1", "core_siblings": "1",
		"cluster_cpus": "1", "die_cpus": "1", "package_cpus": "1",
	}
	for name, content := range files {
		v.mkGenFile(topology, name, func() []byte { return []byte(content + "\n") })
	}
	return root
}
