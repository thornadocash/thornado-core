package blame

import (
	. "gopkg.in/check.v1"
)

type BlameTestSuite struct{}

var _ = Suite(&BlameTestSuite{})

func createNewNode(key string) Node {
	return Node{
		Pubkey:         key,
		BlameData:      nil,
		BlameSignature: nil,
	}
}

func (BlameTestSuite) TestBlame(c *C) {
	b := NewBlame("whatever", []Node{createNewNode("1"), createNewNode("2")})
	c.Assert(b.FailReason, HasLen, 8)
	c.Logf("%s", b)
	b.AddBlameNodes(createNewNode("3"), createNewNode("4"))
	c.Assert(b.BlameNodes, HasLen, 4)
	b.AddBlameNodes(createNewNode("3"))
	c.Assert(b.BlameNodes, HasLen, 4)
	b.SetBlame("helloworld", nil, false, "round5")
	c.Assert(b.FailReason, Equals, "helloworld")
	c.Assert(b.Round, Equals, "round5")
}

func (BlameTestSuite) TestAlreadyBlame(c *C) {
	b := NewBlame("test", nil)
	c.Assert(b.AlreadyBlame(), Equals, false)

	b.AddBlameNodes(createNewNode("node1"))
	c.Assert(b.AlreadyBlame(), Equals, true)
}

func (BlameTestSuite) TestNodeEqual(c *C) {
	n1 := NewNode("pk1", []byte("data1"), []byte("sig1"))
	n2 := NewNode("pk1", []byte("data2"), []byte("sig1"))
	n3 := NewNode("pk2", []byte("data1"), []byte("sig1"))

	c.Assert(n1.Equal(n2), Equals, true)
	c.Assert(n1.Equal(n3), Equals, false)
}
