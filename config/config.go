package config

type MIB struct {
	Directory   []string `yaml:"directory"`
	LoadModules []string `yaml:"modules"`
}

type Target struct {
	IPAddress string `yaml:"ipaddress"`
	Community string `yaml:"community"`
	Oid       string `yaml:"oid"`
}

type MetricValue struct {
	Name  string `yaml:"name"`
	Label string `yaml:"label"`
	Unit  string `yaml:"unit"`
	Diff  bool   `yaml:"diff"`
}

type Metric struct {
	Prefix string        `yaml:"prefix"`
	Key    string        `yaml:"key"`
	Value  []MetricValue `yaml:"value"`
}

type Config struct {
	MIB    MIB      `yaml:"mib"`
	Target Target   `yaml:"target"`
	Metric []Metric `yaml:"metric"`
	Prefix string   `yaml:"prefix"`
}
