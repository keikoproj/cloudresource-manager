#!/bin/bash
set -e
# Any subsequent(*) commands which fail will cause the shell script to exit immediately

helpFunction()
{
   echo ""
   echo "Usage: $0 -a awsAccountNumber -c clusterName"
   echo "\t-a awsAccountNumber refers to the account number of the source AWS account"
   echo "\t-c clusterName refers to the name of the cluster running the controller in your source AWS account"
   exit 1 # Exit script after printing help
}

while getopts "a:c:" opt
do
   case "$opt" in
      a ) awsAccountNumber="$OPTARG" ;;
      c ) clusterName="$OPTARG" ;;
      ? ) helpFunction ;; # Print helpFunction in case parameter is non-existent
   esac
done

# Print helpFunction in case parameters are empty
# Begin execution of remaining script only if all parameters are correct
if [ -z "$awsAccountNumber" ] || [ -z "$clusterName" ]
then
   echo "Some or all of the parameters are empty";
   helpFunction
fi

# Display the input arguments
echo "Source AWS account number : $awsAccountNumber"
echo "Cluster Name: $clusterName"

# fetch user credentials to generate temporary AWS credentials
export AWS_PROFILE=default

# generating k8s_assume_role
echo "Creating k8s_assume_role ..."
# replace the rolename with kiam-role used in your cluster.
masterRoleName="${clusterName}-kiam-role"
cp resources/source_account_trust_relationship.json /tmp/k8s_assume_role_trust_relationship.json
sed -i '' -e "s/<accnt>/${awsAccountNumber}/g" -e "s/<master_role>/${masterRoleName}/g" /tmp/k8s_assume_role_trust_relationship.json
aws iam create-role --role-name k8s_assume_role --assume-role-policy-document file:///tmp/k8s_assume_role_trust_relationship.json

echo "Creating inline permission policy k8s_assume_policy and attaching it to k8s_assume_role ..."
aws iam put-role-policy --role-name k8s_assume_role --policy-name k8s_assume_policy --policy-document file://resources/k8s_assume_policy.json
echo "Policy attached successfully"
echo "Role creation completed successfully!"