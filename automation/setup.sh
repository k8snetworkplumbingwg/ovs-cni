# Prepare environment for testing and automation.
#
# source automation/setup.sh
# cd ${TMP_PROJECT_PATH}

echo 'Copy the project under a temporary'
TMP_PROJECT_PATH=/tmp/src/github.com/k8snetworkplumbingwg/ovs-cni
rm -rf ${TMP_PROJECT_PATH}
mkdir -p ${TMP_PROJECT_PATH}
cp -rf $(pwd)/. ${TMP_PROJECT_PATH}

echo 'Exporting temporary project path'
export TMP_PROJECT_PATH
