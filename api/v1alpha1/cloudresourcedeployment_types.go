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

package v1alpha1

import (
	"fmt"
	"hash/adler32"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CloudResourceDeploymentSpec defines the desired state of CloudResourceDeployment
type CloudResourceDeploymentSpec struct {
	// Important: Run "make" to regenerate code after modifying this file
	Cloudformation *StackSpec `json:"cloudformation,omitempty" yaml:"cloudformation,omitempty"`
}

// StackSpec defines the desired state of Stack
type StackSpec struct {
	// Important: Run "make" to regenerate code after modifying this file
	Parameters map[string]string  `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Tags       map[string]string  `json:"tags,omitempty" yaml:"tags,omitempty"`
	Template   string             `json:"template,omitempty" yaml:"template,omitempty"`
	Stackname  string             `json:"stackname,omitempty" yaml:"stackname,omitempty"`
	CARole     AssumeRoleProvider `json:"assumeroleprovider,omitempty" yaml:"assumeroleprovider,omitempty"`
	Timeout    int                `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Region     string             `json:"region,omitempty" yaml:"region,omitempty"`
}

// CloudResourceDeploymentStatus defines the observed state of CloudResourceDeployment
type CloudResourceDeploymentStatus struct {
	// Important: Run "make" to regenerate code after modifying this file
	Status        string       `json:"status,omitempty"`
	Message       string       `json:"message,omitempty"`
	Checksum      string       `json:"checksum"`
	Changesetname string       `json:"changesetname"`
	StartedAt     *metav1.Time `json:"startedAt,omitempty"`
}

// +kubebuilder:object:root=true

// CloudResourceDeployment is the Schema for the cloudresourcedeployments API
type CloudResourceDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudResourceDeploymentSpec   `json:"spec,omitempty"`
	Status CloudResourceDeploymentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudResourceDeploymentList contains a list of CloudResourceDeployment
type CloudResourceDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudResourceDeployment `json:"items"`
}

type AssumeRoleProvider struct {
	// credentials.Expiry `json:"expiry,omitempty"`

	// STS client to make assume role request with.
	// Client AssumeRoler `json:"assumeroler,omitempty"`

	// Role to be assumed.
	RoleARN string `json:"rolearn,omitempty" yaml:"rolearn,omitempty"`

	// Session name, if you wish to reuse the credentials elsewhere.
	RoleSessionName string `json:"roleSessionName,omitempty" yaml:"roleSessionName,omitempty"`

	// ServiceRole that Cloudformation will assume. This is to have more security.
	ServiceRoleARN string `json:"servicerolearn,omitempty" yaml:"servicerolearn,omitempty"`

	// Optional, you can pass tag key-value pairs to your session. These tags are called session tags.
	// Tags []sts.Tag `json:"tags,omitempty"`

	// A list of keys for session tags that you want to set as transitive.
	// If you set a tag key as transitive, the corresponding key and value passes to subsequent sessions in a role chain.
	// TransitiveTagKeys []string `json:"transitiveTagKeys,omitempty"`

	// Expiry duration of the STS credentials. Defaults to 15 minutes if not set.
	Duration time.Duration `json:"duration,omitempty" yaml:"duration,omitempty"`

	// Optional ExternalID to pass along, defaults to nil if not set.
	ExternalID string `json:"externalID,omitempty" yaml:"externalID,omitempty"`

	// Optional AccountID, Here we would only accept the account number in the implementation and we would construct the
	// rolearn dynamically to keep it secure.
	AccountID string `json:"acctnum,omitempty" yaml:"acctnum,omitempty"`

	//  Optional Environment, this field is required (along with AccountId) to
	// generate the ExternalID in the scenario where it is not passed explicitly.
	Environment string `json:"env" yaml:"env"`

	// The policy plain text must be 2048 bytes or shorter. However, an internal
	// conversion compresses it into a packed binary format with a separate limit.
	// The PackedPolicySize response element indicates by percentage how close to
	// the upper size limit the policy is, with 100% equaling the maximum allowed
	// size.
	Policy string `json:"policy,omitempty" yaml:"policy,omitempty"`

	// The identification number of the MFA device that is associated with the user
	// who is making the AssumeRole call. Specify this value if the trust policy
	// of the role being assumed includes a condition that requires MFA authentication.
	// The value is either the serial number for a hardware device (such as GAHT12345678)
	// or an Amazon Resource Name (ARN) for a virtual device (such as arn:aws:iam::123456789012:mfa/user).
	SerialNumber string `json:"serialNumber,omitempty" yaml:"serialNumber,omitempty"`

	// The value provided by the MFA device, if the trust policy of the role being
	// assumed requires MFA (that is, if the policy includes a condition that tests
	// for MFA). If the role being assumed requires MFA and if the TokenCode value
	// is missing or expired, the AssumeRole call returns an "access denied" error.
	//
	// If SerialNumber is set and neither TokenCode nor TokenProvider are also
	// set an error will be returned.
	// TokenCode string `json:"tokencode,omitempty"`

	// Async method of providing MFA token code for assuming an IAM role with MFA.
	// The value returned by the function will be used as the TokenCode in the Retrieve
	// call. See StdinTokenProvider for a provider that prompts and reads from stdin.
	//
	// This token provider will be called when ever the assumed role's
	// credentials need to be refreshed when SerialNumber is also set and
	// TokenCode is not set.
	//
	// If both TokenCode and TokenProvider is set, TokenProvider will be used and
	// TokenCode is ignored.
	// TokenProvider func() (string, error)

	// ExpiryWindow will allow the credentials to trigger refreshing prior to
	// the credentials actually expiring. This is beneficial so race conditions
	// with expiring credentials do not cause request to fail unexpectedly
	// due to ExpiredTokenException exceptions.
	//
	// So a ExpiryWindow of 10s would cause calls to IsExpired() to return true
	// 10 seconds before the credentials are actually expired.
	//
	// If ExpiryWindow is 0 or less it will be ignored.
	ExpiryWindow time.Duration `json:"expirywindow,omitempty" yaml:"expirywindow,omitempty"`

	// MaxJitterFrac reduces the effective Duration of each credential requested
	// by a random percentage between 0 and MaxJitterFraction. MaxJitterFrac must
	// have a value between 0 and 1. Any other value may lead to expected behavior.
	// With a MaxJitterFrac value of 0, default) will no jitter will be used.
	//
	// For example, with a Duration of 30m and a MaxJitterFrac of 0.1, the
	// AssumeRole call will be made with an arbitrary Duration between 27m and
	// 30m.
	//
	// MaxJitterFrac should not be negative.
	// MaxJitterFrac float64 `json:"maxJitterfrac,omitempty"`
}

func init() {
	SchemeBuilder.Register(&CloudResourceDeployment{}, &CloudResourceDeploymentList{})
}

// CalculateChecksum converts the AddonSpec into a hash string (using Alder32 algo)
func (c *CloudResourceDeployment) CalculateChecksum() string {
	return fmt.Sprintf("%x", adler32.Checksum([]byte(fmt.Sprintf("%+v", c.Spec.Cloudformation))))
}
