package constants

type DummyConfigs struct {
	int64values  map[ConfigName]int64
	boolValues   map[ConfigName]bool
	stringValues map[ConfigName]string
}

// NewDummyConfigs create a new instance of DummyConfigs for test purpose
func NewDummyConfigs(int64Values map[ConfigName]int64, boolValues map[ConfigName]bool, stringValues map[ConfigName]string) *DummyConfigs {
	return &DummyConfigs{
		int64values:  int64Values,
		boolValues:   boolValues,
		stringValues: stringValues,
	}
}

func (dc *DummyConfigs) GetInt64Value(name ConfigName) int64 {
	v, ok := dc.int64values[name]
	if !ok {
		return 0
	}
	return v
}

func (dc *DummyConfigs) GetBoolValue(name ConfigName) bool {
	v, ok := dc.boolValues[name]
	if !ok {
		return false
	}
	return v
}

func (dc *DummyConfigs) GetStringValue(name ConfigName) string {
	v, ok := dc.stringValues[name]
	if !ok {
		return ""
	}
	return v
}

func (dc *DummyConfigs) String() string {
	return ""
}

func (dc *DummyConfigs) GetConfigValsByKeyname() ConfigValsByKeyname {
	return ConfigValsByKeyname{}
}
