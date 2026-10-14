package ec2

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestFormatRules(t *testing.T) {
	got := formatRules([]types.IpPermission{
		{IpProtocol: aws.String("tcp"), FromPort: aws.Int32(22), ToPort: aws.Int32(22),
			IpRanges: []types.IpRange{{CidrIp: aws.String("10.0.0.0/8")}}},
		{IpProtocol: aws.String("tcp"), FromPort: aws.Int32(8000), ToPort: aws.Int32(8080),
			UserIdGroupPairs: []types.UserIdGroupPair{{GroupId: aws.String("sg-123")}}},
		{IpProtocol: aws.String("-1"), IpRanges: []types.IpRange{{CidrIp: aws.String("0.0.0.0/0")}}},
	})
	for _, want := range []string{"tcp   22          10.0.0.0/8", "tcp   8000-8080   sg-123", "all   all         0.0.0.0/0"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if formatRules(nil) != "-" {
		t.Error("empty rules should render as -")
	}
}

func TestStateLabel(t *testing.T) {
	if got := stateLabel(&types.InstanceState{Name: types.InstanceStateNameRunning}); got != "● running" {
		t.Errorf("got %q", got)
	}
	if got := stateLabel(nil); got != "-" {
		t.Errorf("got %q", got)
	}
}
