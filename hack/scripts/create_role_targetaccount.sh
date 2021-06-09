#!/bin/bash
set -e
# Any subsequent(*) commands which fail will cause the shell script to exit immediately

helpFunction()
{
   echo ""
   echo "Usage: $0 -s sourceAwsAccountNumber -t targetAwsAccountNumber -e environment"
   echo "\t-s sourceAwsAccountNumber refers to the account number of the source AWS account running the controller"
   echo "\t-t targetAwsAccountNumberv refers to the account number of the target AWS account in which the Cfn needs to be executed"
   echo "\t-e environment"
   exit 1 # Exit script after printing help
}

while getopts "s:t:b:g:e:" opt
do
   case "$opt" in
      s ) sourceAwsAccountNumber="$OPTARG" ;;
      t ) targetAwsAccountNumber="$OPTARG" ;;
      e ) environment="$OPTARG" ;;
      ? ) helpFunction ;; # Print helpFunction in case parameter is non-existent
   esac
done

# Print helpFunction in case parameters are empty
# Begin execution of remaining script only if all parameters are correct
if [ -z "$sourceAwsAccountNumber" ] || [ -z "$targetAwsAccountNumber" ] || [ -z "$environment" ]
then
   echo "Some or all of the parameters are empty";
   helpFunction
fi

# Display the input arguments
echo "Source AWS account number : $sourceAwsAccountNumber"
echo "Target AWS account number : $targetAwsAccountNumber"
echo "Environment : $environment"

# fetch user credentials to generate temporary AWS credentials
echo "Generating AWS credentials..."
export AWS_PROFILE=default

# generating clf-service-role
echo "Creating clf-service-role ..."
aws iam create-role --role-name clf-service-role --assume-role-policy-document file://resources/service_role_trust_relationship.json

echo "Creating inline permission policy clf_service_policy and attaching it to clf-service-role ..."
aws iam put-role-policy --role-name clf-service-role --policy-name clf_service_policy --policy-document file://resources/power_user_policy.json
echo "Policy attached successfully"
echo "clf-service-role creation completed successfully!"

echo "Creating cfn-master-role ..."
cp resources/trust_relationship_template.json /tmp/cfn-master-role_trust_relationship.json
sed -i '' -e "s/<accnt>/${sourceAwsAccountNumber}/g" -e "s/<master_role>/k8s_assume_role/g" -e "s/<external_id>/cloudresource-${targetAwsAccountNumber}-${environment}/g" /tmp/cfn-master-role_trust_relationship.json
aws iam create-role --role-name cfn-master-role --assume-role-policy-document file:///tmp/cfn-master-role_trust_relationship.json

# generating cfn_master_policy
echo "Generating cfn_master_policy ..."
cp resources/cfn_master_policy.json /tmp/cfn_master_policy.json
sed -i '' -e "s/<account>/${targetAwsAccountNumber}/g" -e "s/<servicerole>/clf-service-role/g" /tmp/cfn_master_policy.json
cat /tmp/cfn_master_policy.json

echo "Creating inline permission policy cfn_master_policy and attaching it to cfn-master-role ..."
aws iam put-role-policy --role-name cfn-master-role --policy-name cfn_master_policy --policy-document file:///tmp/cfn_master_policy.json
echo "Policy attached successfully"
echo "cfn-master-role creation completed successfully!"
