package constants

import (
	"regexp"

	"github.com/blang/semver"
	. "gopkg.in/check.v1"
)

type ConstantsTestSuite struct{}

var _ = Suite(&ConstantsTestSuite{})

func (ConstantsTestSuite) TestConfigName_String(c *C) {
	constantNames := []ConfigName{
		Chain_BlockTimeSeconds,
		Vault_BaseMembersMin,
		Node_SetDesired,
		Churn_IntervalMinutes,
		Observation_MissPenaltyPoints,
		Keysign_PeriodMinutes,
		DoubleSign_MaxAgeMinutes,
		Node_BadPenaltyPointsMin,
	}
	for _, item := range constantNames {
		c.Assert(item.String(), Not(Equals), "NA")
	}
}

func (ConstantsTestSuite) TestGetConfigValues(c *C) {
	ver := semver.MustParse("0.0.9")
	c.Assert(GetConfigValues(ver), NotNil)
	c.Assert(GetConfigValues(SWVersion), NotNil)
}

func (ConstantsTestSuite) TestAllConfigName(c *C) {
	keyRegex := regexp.MustCompile(ConfigKeyRegex).MatchString
	for i := 0; i < len(_ConfigName_index)-1; i++ {
		key := ConfigName(i)
		if !keyRegex(key.String()) {
			c.Errorf("key:%s can't be used to set config", key)
		}
	}
}
