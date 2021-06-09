<h1> Roles and Permissions </h1>

Each account managed by GitOps for AWS will need to be properly configured with the required policies/roles to allow the CloudResource controller running on the Source cluster to manage stacks in the target AWS account. To support this, the Cloud Resource controller pods will have a role attached whose policy enables it to assume the cfn-master-role on the target account. The cfn-master-role has the required permissions on CloudFormations stacks which enables the CloudResource controller to create, update, delete and get the stack status. However,  for changeset creation and execution, the CloudResource controller leverages the clf-service-role, which has the privileges to create/update/delete the resources handled by the stack. The CloudResource controller cannot assume this clf-service-role directly, it gains access to it through the cfn-master-role (leveraging the iam:PassRole permission). As a security measure, the cfn-master-role can only be assumed by the k8s_assume_role on the Source account and it has also been equipped with an externalId to prevent the <a href="https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_create_for-user_externalid.html">Confused Deputy problem</a>. There also exists a manny-s3-role which provides read only access to the chapi artifacts stored in S3.

<table>
<thead>
  <tr>
    <th></th>
    <th>k8s_assume_role</th>
    <th>cfn-master-role</th>
    <th>clf-service-role</th>
    <th>manny-s3-role</th>
  </tr>
</thead>
<tbody>
  <tr>
    <td><b> AWS Account on <br>which it exists</td>
    <td>Source account</td>
    <td>Target account</td>
    <td>Target account</td>
    <td>Source account</td>
  </tr>
  <tr>
    <td><b>Policies <br>[Access to]<br></td>
    <td>- sts:AssumeRole on roles in <br>any account that have the <br>name “cfn-master-role”</td>
    <td>- iam:GetRole<br>- iam:PassRole<br>- CloudFormation</td>
    <td>- [Power user access]<br> - iam:GetRole<br> - iam:CreateRole<br> - iam:DeleteRole<br> - iam:PutRolePolicy<br> - iam:getRolePolicy<br> - iam:PassRole<br> - iam:DeleteRolePolicy<br> - iam:CreateServiceLinkedRole<br> - iam:DeleteServiceLinkedRole<br> - iam:ListRoles<br> - organizations:DescribeOrganization<br> - account:ListRegions<br></td>
    <td>- s3:Get*<br>- s3:List*</td>
  </tr>
  <tr>
    <td><b>Trust Relationships</td>
    <td>Source cluster Master role</td>
    <td>Can be assumed only by <br>pods with k8s_assume_role<br> in Source account<br></td>
    <td>cloudformation.aws.com</td>
    <td>Source cluster Master role</td>
  </tr>
</tbody>
</table>

The creation of the above roles has been automated. 

<b>Usage :</b>

On Source AWS account (one time activity)

&nbsp;&nbsp;&nbsp;&nbsp; sh create_role_sourceaccount.sh -a \<account number\> -c \<cluster name\>

On target AWS account

&nbsp;&nbsp;&nbsp;&nbsp; sh create_role_targetaccount.sh -s \<Source account number\> -t \<target account number\> -e \<environment\>

