/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package arczonalshift_test

import (
	"fmt"
	"math/rand"
	"testing"

	arczonalshiftservice "github.com/aws/aws-sdk-go-v2/service/arczonalshift"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	v1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	"github.com/aws/karpenter-provider-aws/pkg/operator/options"
	"github.com/aws/karpenter-provider-aws/pkg/test"
	environmentaws "github.com/aws/karpenter-provider-aws/test/pkg/environment/aws"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

var env *environmentaws.Environment
var nodeClass *v1.EC2NodeClass
var nodePool *karpv1.NodePool

func TestZonalShift(t *testing.T) {
	RegisterFailHandler(Fail)
	BeforeSuite(func() {
		env = environmentaws.NewEnvironment(t)
	})
	AfterSuite(func() {
		env.Stop()
	})
	RunSpecs(t, "ZonalShift")
}

var _ = BeforeEach(func() {
	env.Context = options.ToContext(env.Context, test.Options(test.OptionsFields{
		EnableZonalShift: lo.ToPtr(true),
	}))
	env.BeforeEach()
	nodeClass = env.DefaultEC2NodeClass()
	nodePool = env.DefaultNodePool(nodeClass)
})
var _ = AfterEach(func() { env.Cleanup() })
var _ = AfterEach(func() { env.AfterEach() })

var _ = Describe("Zonal Shift", func() {
	var clusterArn string
	var zoneid string
	var zonalshiftid *string
	var subnetInfo []environmentaws.SubnetInfo
	BeforeEach(func() {
		clusterArn = fmt.Sprintf("arn:aws:eks:%s:%s:cluster/%s", env.Region, env.ExpectAccountID(), env.ClusterName)
		subnetInfo = lo.UniqBy(env.GetSubnetInfo(map[string]string{"karpenter.sh/discovery": env.ClusterName}), func(s environmentaws.SubnetInfo) string {
			return s.Zone
		})
		zoneid = subnetInfo[rand.Intn(len(subnetInfo))].ZoneID //nolint:gosec
	})
	It("Resource should be registered", func() {
		By("making a successful GetManagedResource API call")
		env.ExpectRegisteredToZonalShift(
			env.Context,
		)
	})
	It("should update cache when a zonal shift is detected", func() {
		By("using the Zonal Shift Provider to check zonal shift status")

		startzonalshiftresponse, err := env.ARCZONALSHIFTAPI.StartZonalShift(env.Context, &arczonalshiftservice.StartZonalShiftInput{
			ResourceIdentifier: lo.ToPtr(clusterArn),
			AwayFrom:           lo.ToPtr(zoneid),
			ExpiresIn:          lo.ToPtr("1h"),
			Comment:            lo.ToPtr("karpenter e2e test"),
		})
		zonalshiftid = startzonalshiftresponse.ZonalShiftId
		Expect(err).To(BeNil())
		env.EventuallyExpectClusterToZonalShift(zoneid)
	})
	//It("should not scale deployments into a shifted zone", func() {
	//	By("not scheduling pods into the shifted zone")
	//	// Each pod specifies a requirement on this expected zone, where the value is the matching zone for the
	//	// required zone-id. This allows us to verify that Karpenter launched the node in the correct zone, even if
	//	// it doesn't add the zone-id label and the label is added by CCM. If we didn't take this approach, we would
	//	// succeed even if Karpenter doesn't add the label and /or incorrectly generated offerings on k8s 1.30 and
	//	// above. This is an unlikely scenario, and adding this check is a defense in depth measure.
	//	const expectedZoneLabel = "expected-zone-label"
	//	coretest.ReplaceRequirements(nodePool, karpv1.NodeSelectorRequirementWithMinValues{
	//		Key:      expectedZoneLabel,
	//		Operator: corev1.NodeSelectorOpExists,
	//	})
	//	// Need to label pods with zoneid to make asserting which ones should be created vs pending easier.
	//	pods := lo.Map(subnetInfo, func(info environmentaws.SubnetInfo, _ int) *corev1.Pod {
	//		labels := map[string]string{
	//			"zoneid": info.ZoneID,
	//		}
	//		return coretest.Pod(coretest.PodOptions{
	//			ObjectMeta: metav1.ObjectMeta{Labels: labels},
	//			NodeRequirements: []corev1.NodeSelectorRequirement{
	//				{
	//					Key:      expectedZoneLabel,
	//					Operator: corev1.NodeSelectorOpIn,
	//					Values:   []string{info.Zone},
	//				},
	//				{
	//					Key:      v1.LabelTopologyZoneID,
	//					Operator: corev1.NodeSelectorOpIn,
	//					Values:   []string{info.ZoneID},
	//				},
	//			},
	//		})
	//	})
	//	for _, pod := range pods {
	//		// Pods that are in the shifted zone are expected to be pending
	//		if pod.ObjectMeta.Labels["zoneid"] == zoneid {
	//			env.ConsistentlyExpectPendingPods(5, pod)
	//		} else {
	//			env.ExpectCreated(pod)
	//		}
	//	}
	//})
	It("should update cache when a zonal shift is ended", func() {
		By("using the Zonal Shift Provider to check zonal shift status")

		_, err := env.ARCZONALSHIFTAPI.CancelZonalShift(env.Context, &arczonalshiftservice.CancelZonalShiftInput{
			ZonalShiftId: zonalshiftid,
		})
		Expect(err).To(BeNil())
		env.EventuallyExpectClusterToNotHaveZonalShift(zoneid)
	})
	//It("should scale deployments into zones after shift ends", func() {
	//	By("scheduling pods into the previously shifted zone")
	//	// Each pod specifies a requirement on this expected zone, where the value is the matching zone for the
	//	// required zone-id. This allows us to verify that Karpenter launched the node in the correct zone, even if
	//	// it doesn't add the zone-id label and the label is added by CCM. If we didn't take this approach, we would
	//	// succeed even if Karpenter doesn't add the label and /or incorrectly generated offerings on k8s 1.30 and
	//	// above. This is an unlikely scenario, and adding this check is a defense in depth measure.
	//	const expectedZoneLabel = "expected-zone-label"
	//	coretest.ReplaceRequirements(nodePool, karpv1.NodeSelectorRequirementWithMinValues{
	//		Key:      expectedZoneLabel,
	//		Operator: corev1.NodeSelectorOpExists,
	//	})
	//	pods := lo.Map(subnetInfo, func(info environmentaws.SubnetInfo, _ int) *corev1.Pod {
	//		labels := map[string]string{
	//			"zoneid": info.ZoneID,
	//		}
	//		return coretest.Pod(coretest.PodOptions{
	//			ObjectMeta: metav1.ObjectMeta{Labels: labels},
	//			NodeRequirements: []corev1.NodeSelectorRequirement{
	//				{
	//					Key:      expectedZoneLabel,
	//					Operator: corev1.NodeSelectorOpIn,
	//					Values:   []string{info.Zone},
	//				},
	//				{
	//					Key:      v1.LabelTopologyZoneID,
	//					Operator: corev1.NodeSelectorOpIn,
	//					Values:   []string{info.ZoneID},
	//				},
	//			},
	//		})
	//	})
	//	for _, pod := range pods {
	//		env.ExpectCreated(pod)
	//	}
	//})
})
