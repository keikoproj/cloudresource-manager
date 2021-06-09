Filling the Assumeroleprovider section of the config file

<b>NOTE:</b> env is a compulsory field

<ol>
  <li><b> Option 1 </b></li>
  If you pass the acctnum, other fields such as externalID, rolearn, servicerolearn are not required to be passed. 
  
  Eg

  ```
  env: ppd
  acctnum: '123456789011'
  ```
  
  <li><b> Option 2 </b></li>
  If you prefer not to pass the acctnum, you would then be required to pass the rolearn explicitly, from which the externalID and servicerolearn are deduced
  
  Eg
  ```
  env: ppd
  rolearn: 'arn:aws:iam::123456789011:role/cfn-master-role'
  ```
</ol>
