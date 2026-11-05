package three

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

// Captured before adding PointLightList; pin the original single-point shader.
func TestPointLightLegacyHashes(t *testing.T) {
	want := map[string]string{
		"constant/ssaa1":        "947aff8d249d054ecd22dddd9939dc690f49fbcd3324fc2fa1edd9a5810c57e0",
		"constant/ssaa2":        "4d05b8b32e07a586cc18013b4c43bc284397a9e55dab2b17b3354a92729249f7",
		"constant/ssaa4":        "d64126ab0450240550dfed97ce316dea5a8c604b95f7b66e7c354003657a4adf",
		"opaque-bilinear/ssaa1": "ef64636a2253615b63dc456bde7cc86ec20902b85da3f9af6a7763751b6863a4",
		"opaque-bilinear/ssaa2": "b6c281d6f421200bef924878950ec9f3de0d65df183c477eb0a9505fb6d83ee7",
		"opaque-bilinear/ssaa4": "16d8b902a28cf1e57ffd6a4a72387e67e7740e087b6d02ecd01df0bb813e3bd7",
		"tinted-bilinear/ssaa1": "4081622d229a40180be39d84057665ea11e2f8ac62e318019bac2de9db394e26",
		"tinted-bilinear/ssaa2": "9d83cbc1728ebd3358ea402b7646fed451a097c68ac116646e11843437075214",
		"tinted-bilinear/ssaa4": "bc8e3a915b1c113a8289b0830410497eab35bf3ec3174d955c40d4546fa3cfc4",
	}
	for _, mode := range []string{"constant", "opaque-bilinear", "tinted-bilinear"} {
		for _, scale := range []int{1, 2, 4} {
			cfg, camera, instances := pointLightRayFixture(t, mode, true)
			cfg.Supersampling = scale
			img := mustShadowRender(t, cfg, camera, instances...)
			name := fmt.Sprintf("%s/ssaa%d", mode, scale)
			got := fmt.Sprintf("%x", sha256.Sum256(img.Pix))
			if got != want[name] {
				t.Fatalf("single point %s: %s want %s", name, got, want[name])
			}
		}
	}
}
