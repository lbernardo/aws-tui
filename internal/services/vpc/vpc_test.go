package vpc

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestFormatRoutes(t *testing.T) {
	got := formatRoutes([]types.Route{
		{DestinationCidrBlock: aws.String("10.0.0.0/16"), GatewayId: aws.String("local")},
		{DestinationCidrBlock: aws.String("0.0.0.0/0"), NatGatewayId: aws.String("nat-1"), State: types.RouteStateBlackhole},
	})
	for _, want := range []string{"10.0.0.0/16          → local", "0.0.0.0/0            → nat-1", "blackhole"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
