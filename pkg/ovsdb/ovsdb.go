// Modifications copyright (C) 2017 Che Wei, Lin
// Copyright 2014 Cisco Systems Inc. All rights reserved.
// Copyright 2019 Red Hat Inc. All rights reserved.

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ovsdb

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"

	"github.com/ovn-org/libovsdb/client"
	"github.com/ovn-org/libovsdb/model"
	"github.com/ovn-org/libovsdb/ovsdb"
)

const ovsPortOwner = "ovs-cni.network.kubevirt.io"
const (
	bridgeTable = "Bridge"
	ovsTable    = "Open_vSwitch"
)

// Bridge defines an object in Bridge table
type Bridge struct {
	UUID string `ovsdb:"_uuid"`
}

// OpenvSwitch defines an object in Open_vSwitch table
type OpenvSwitch struct {
	UUID string `ovsdb:"_uuid"`
}

// OvsDriver OVS driver state
type OvsDriver struct {
	// OVS client
	ovsClient client.Client
}

// OvsBridgeDriver OVS bridge driver state
type OvsBridgeDriver struct {
	OvsDriver

	// Name of the OVS bridge
	OvsBridgeName string
}

// constants used to identify if a mirror is a comsumer or a producer
const (
	MirrorProducer = iota
	MirrorConsumer
)

// connectToOvsDb connect to ovsdb
func connectToOvsDb(ovsSocket string) (client.Client, error) {
	dbmodel, err := model.NewDBModel("Open_vSwitch",
		map[string]model.Model{bridgeTable: &Bridge{}, ovsTable: &OpenvSwitch{}})
	if err != nil {
		return nil, fmt.Errorf("unable to create DB model error: %v", err)
	}

	ovsDB, err := client.NewOVSDBClient(dbmodel, client.WithEndpoint(ovsSocket))
	if err != nil {
		return nil, fmt.Errorf("unable to create DB client error: %v", err)
	}
	err = ovsDB.Connect(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ovsdb error: %v", err)
	}

	return ovsDB, nil
}

// NewOvsDriver Create a new OVS driver with Unix socket
func NewOvsDriver(ovsSocket string) (*OvsDriver, error) {
	ovsDriver := new(OvsDriver)

	ovsDB, err := connectToOvsDb(ovsSocket)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ovsdb error: %v", err)
	}

	ovsDriver.ovsClient = ovsDB

	return ovsDriver, nil
}

// NewOvsBridgeDriver Create a new OVS driver for a bridge with Unix socket
func NewOvsBridgeDriver(bridgeName, socketFile string) (*OvsBridgeDriver, error) {
	ovsDriver := new(OvsBridgeDriver)

	if socketFile == "" {
		socketFile = "unix:/var/run/openvswitch/db.sock"
	}

	ovsDB, err := connectToOvsDb(socketFile)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ovsdb socket %s: error: %v", socketFile, err)
	}

	// Setup state
	ovsDriver.ovsClient = ovsDB
	ovsDriver.OvsBridgeName = bridgeName

	bridgeExist, err := ovsDriver.IsBridgePresent(bridgeName)
	if err != nil {
		return nil, err
	}

	if !bridgeExist {
		return nil, fmt.Errorf("failed to find bridge %s", bridgeName)
	}

	// Return the new OVS driver
	return ovsDriver, nil
}

// Wrapper for ovsDB transaction
func (ovsd *OvsDriver) ovsdbTransact(ops []ovsdb.Operation) ([]ovsdb.OperationResult, error) {
	// Perform OVSDB transaction
	reply, _ := ovsd.ovsClient.Transact(ops...)

	if len(reply) < len(ops) {
		return nil, errors.New("OVS transaction failed. Less replies than operations")
	}

	// Parse reply and look for errors
	for _, o := range reply {
		if o.Error != "" {
			return nil, errors.New("OVS Transaction failed err " + o.Error + " Details: " + o.Details)
		}
	}

	// Return success
	return reply, nil
}

// **************** OVS driver API ********************

// CreatePort Create an internal port in OVS
func (ovsd *OvsBridgeDriver) CreatePort(intfName, contNetnsPath, contIfaceName, ovnPortName string, ofportRequest uint, vlanTag uint, trunks []uint, portType string, intfType string) error {
	intfUUID, intfOp, err := createInterfaceOperation(intfName, ofportRequest, ovnPortName, intfType)
	if err != nil {
		return err
	}

	portUUID, portOp, err := createPortOperation(intfName, contNetnsPath, contIfaceName, vlanTag, trunks, portType, intfUUID)
	if err != nil {
		return err
	}

	mutateOp := attachPortOperation(portUUID, ovsd.OvsBridgeName)

	// Perform OVS transaction
	operations := []ovsdb.Operation{*intfOp, *portOp, *mutateOp}

	_, err = ovsd.ovsdbTransact(operations)
	return err
}

// DeletePort Delete a port from OVS
func (ovsd *OvsBridgeDriver) DeletePort(intfName string) error {
	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, intfName)
	row, err := ovsd.findByCondition("Port", condition, nil)
	if err != nil {
		return err
	}

	externalIDs, err := getExternalIDs(row)
	if err != nil {
		return fmt.Errorf("get external ids: %v", err)
	}
	if externalIDs["owner"] != ovsPortOwner {
		return fmt.Errorf("port not created by ovs-cni")
	}

	// We make a select transaction using the interface name
	// Then get the Port UUID from it
	portUUID := row["_uuid"].(ovsdb.UUID)

	intfOp := deleteInterfaceOperation(intfName)

	portOp := deletePortOperation(intfName)

	mutateOp := detachPortOperation(portUUID, ovsd.OvsBridgeName)

	// Perform OVS transaction
	operations := []ovsdb.Operation{*intfOp, *portOp, *mutateOp}

	_, err = ovsd.ovsdbTransact(operations)
	return err
}

func getExternalIDs(row map[string]interface{}) (map[string]string, error) {
	rowVal, ok := row["external_ids"]
	if !ok {
		return nil, fmt.Errorf("row does not contain external_ids")
	}

	rowValOvsMap, ok := rowVal.(ovsdb.OvsMap)
	if !ok {
		return nil, fmt.Errorf("not a OvsMap: %T: %v", rowVal, rowVal)
	}

	extIDs := make(map[string]string, len(rowValOvsMap.GoMap))
	for key, value := range rowValOvsMap.GoMap {
		extIDs[key.(string)] = value.(string)
	}
	return extIDs, nil
}

// BridgeList returns available ovs bridge names
func (ovsd *OvsDriver) BridgeList() ([]string, error) {
	selectOp := []ovsdb.Operation{{
		Op:      "select",
		Table:   "Bridge",
		Columns: []string{"name"},
	}}

	transactionResult, err := ovsd.ovsdbTransact(selectOp)
	if err != nil {
		return nil, err
	}

	if len(transactionResult) != 1 {
		return nil, fmt.Errorf("unknow error")
	}

	operationResult := transactionResult[0]
	if operationResult.Error != "" {
		return nil, fmt.Errorf("%s - %s", operationResult.Error, operationResult.Details)
	}

	bridges := []string{}
	for _, bridge := range operationResult.Rows {
		bridges = append(bridges, fmt.Sprintf("%v", bridge["name"]))
	}

	return bridges, nil
}

// GetOFPortOpState retrieves link state of the OF port
func (ovsd *OvsDriver) GetOFPortOpState(portName string) (string, error) {
	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, portName)
	selectOp := []ovsdb.Operation{{
		Op:      "select",
		Table:   "Interface",
		Columns: []string{"link_state"},
		Where:   []ovsdb.Condition{condition},
	}}

	transactionResult, err := ovsd.ovsdbTransact(selectOp)
	if err != nil {
		return "", err
	}

	if len(transactionResult) != 1 {
		return "", fmt.Errorf("unknown error")
	}

	operationResult := transactionResult[0]
	if operationResult.Error != "" {
		return "", fmt.Errorf("%s - %s", operationResult.Error, operationResult.Details)
	}

	if len(operationResult.Rows) != 1 {
		return "", nil
	}

	return fmt.Sprintf("%v", operationResult.Rows[0]["link_state"]), nil
}

// GetOFPortVlanState retrieves port vlan state of the OF port
func (ovsd *OvsDriver) GetOFPortVlanState(portName string) (string, *uint, []uint, error) {
	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, portName)
	selectOp := []ovsdb.Operation{{
		Op:      "select",
		Table:   "Port",
		Columns: []string{"vlan_mode", "tag", "trunks"},
		Where:   []ovsdb.Condition{condition},
	}}
	var vlanMode = ""
	var tag *uint = nil
	var trunks []uint

	transactionResult, err := ovsd.ovsdbTransact(selectOp)
	if err != nil {
		return vlanMode, tag, trunks, err
	}

	if len(transactionResult) != 1 {
		return vlanMode, tag, trunks, fmt.Errorf("transactionResult length is not one")
	}

	operationResult := transactionResult[0]
	if operationResult.Error != "" {
		return vlanMode, tag, trunks, fmt.Errorf("%s - %s", operationResult.Error, operationResult.Details)
	}

	if len(operationResult.Rows) != 1 {
		return vlanMode, tag, trunks, fmt.Errorf("operationResult.Rows length is not one")
	}

	vlanModeCol := operationResult.Rows[0]["vlan_mode"]
	switch vlanModeCol.(type) {
	case string:
		vlanMode = operationResult.Rows[0]["vlan_mode"].(string)
	}

	tagCol := operationResult.Rows[0]["tag"]
	switch tagCol.(type) {
	case float64:
		tagValue := uint(operationResult.Rows[0]["tag"].(float64))
		tag = &tagValue
	}

	trunksCol := operationResult.Rows[0]["trunks"].(ovsdb.OvsSet).GoSet
	if len(trunksCol) > 0 {
		for i := range trunksCol {
			trunks = append(trunks, uint(trunksCol[i].(float64)))
		}
	}

	return vlanMode, tag, trunks, nil
}

// CreateMirror Creates a new mirror to a specific bridge
func (ovsd *OvsBridgeDriver) CreateMirror(bridgeName, mirrorName string) error {
	mirrorExist, err := ovsd.IsMirrorPresent(mirrorName)
	if err != nil {
		return err
	}

	if !mirrorExist {
		// Insert a Mirror and add it into Bridges
		// as 2 operations in a transaction.
		// The first one returns 'mirrorUUID' to referece the new inserted row
		// in the second operation.
		mirrorUUID, mirrorOp, err := createMirrorOperation(mirrorName)
		if err != nil {
			return err
		}
		attachMirrorOp := attachMirrorOperation(mirrorUUID, bridgeName)

		// Perform OVS transaction
		operations := []ovsdb.Operation{*mirrorOp, *attachMirrorOp}

		_, err = ovsd.ovsdbTransact(operations)
		return err
	}
	return nil
}

// IsMirrorUsed Checks if a mirror of a specific bridge is used (it contains at least a portUUID)
func (ovsd *OvsBridgeDriver) IsMirrorUsed(bridgeName, mirrorName string) (bool, error) {
	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, mirrorName)
	row, err := ovsd.findByCondition("Mirror", condition, nil)
	if err != nil {
		return false, err
	}

	isEmpty, err := isMirrorEmpty(row)
	if err != nil {
		return false, fmt.Errorf("cannot check if mirror %s of bridge %s is empty error: %v", mirrorName, bridgeName, err)
	}

	return !isEmpty, nil
}

// DeleteMirror Removes a mirror of a specific bridge
func (ovsd *OvsBridgeDriver) DeleteMirror(bridgeName, mirrorName string) error {
	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, mirrorName)
	row, err := ovsd.findByCondition("Mirror", condition, nil)
	if err != nil {
		return err
	}

	externalIDs, err := getExternalIDs(row)
	if err != nil {
		return fmt.Errorf("get external ids: %v", err)
	}
	if externalIDs["owner"] != ovsPortOwner {
		return fmt.Errorf("mirror not created by ovs-cni")
	}

	mirrorUUID := row["_uuid"].(ovsdb.UUID)

	deleteOp := deleteMirrorOperation(mirrorName)
	detachFromBridgeOp := detachMirrorFromBridgeOperation(mirrorUUID, bridgeName)

	// Perform OVS transaction
	operations := []ovsdb.Operation{*deleteOp, *detachFromBridgeOp}

	_, err = ovsd.ovsdbTransact(operations)
	return err
}

// AttachPortToMirrorProducer Adds a portUUID as 'select_src_port' or 'select_dst_port' to an existing mirror
// based on ingress and egress values
func (ovsd *OvsBridgeDriver) AttachPortToMirrorProducer(portUUIDStr, mirrorName string, ingress, egress bool) error {
	portUUID := ovsdb.UUID{GoUUID: portUUIDStr}

	if !ingress && !egress {
		return errors.New("a mirror producer must have either a ingress or an egress or both")
	}

	attachPortMirrorOp := attachPortToMirrorProducerOperation(portUUID, mirrorName, ingress, egress)

	// Perform OVS transaction
	operations := []ovsdb.Operation{*attachPortMirrorOp}

	_, err := ovsd.ovsdbTransact(operations)
	return err
}

// AttachPortToMirrorConsumer Adds portUUID as 'output_port' to an existing mirror
func (ovsd *OvsBridgeDriver) AttachPortToMirrorConsumer(portUUIDStr, mirrorName string) error {
	portUUID := ovsdb.UUID{GoUUID: portUUIDStr}

	attachPortMirrorOp := attachPortToMirrorConsumerOperation(portUUID, mirrorName)

	// Perform OVS transaction
	operations := []ovsdb.Operation{*attachPortMirrorOp}

	_, err := ovsd.ovsdbTransact(operations)
	return err
}

// DetachPortFromMirrorProducer Removes portUUID as both 'select_src_port' and 'select_dst_port' from an existing mirror
func (ovsd *OvsBridgeDriver) DetachPortFromMirrorProducer(portUUIDStr, mirrorName string) error {
	portUUID := ovsdb.UUID{GoUUID: portUUIDStr}

	mutateMirrorOp := detachPortFromMirrorOperation(portUUID, mirrorName, MirrorProducer)

	// Perform OVS transaction
	operations := []ovsdb.Operation{*mutateMirrorOp}

	_, err := ovsd.ovsdbTransact(operations)
	return err
}

// DetachPortFromMirrorConsumer Removes portUUID as 'output_port' from an existing mirror
func (ovsd *OvsBridgeDriver) DetachPortFromMirrorConsumer(portUUIDStr, mirrorName string) error {
	portUUID := ovsdb.UUID{GoUUID: portUUIDStr}

	mutateMirrorOp := detachPortFromMirrorOperation(portUUID, mirrorName, MirrorConsumer)

	// Perform OVS transaction
	operations := []ovsdb.Operation{*mutateMirrorOp}

	_, err := ovsd.ovsdbTransact(operations)
	return err
}

// GetMirrorUUID Retrieves the UUID of a mirror from its name
func (ovsd *OvsBridgeDriver) GetMirrorUUID(mirrorName string) (ovsdb.UUID, error) {
	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, mirrorName)
	row, err := ovsd.findByCondition("Mirror", condition, nil)
	if err != nil {
		return ovsdb.UUID{}, err
	}

	// We make a select transaction using the interface name
	// Then get the Mirror UUID from it
	mirrorUUID := row["_uuid"].(ovsdb.UUID)

	return mirrorUUID, nil
}

// GetPortUUID Retrieves the UUID of a port from its name
func (ovsd *OvsBridgeDriver) GetPortUUID(portName string) (ovsdb.UUID, error) {
	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, portName)
	row, err := ovsd.findByCondition("Port", condition, nil)
	if err != nil {
		return ovsdb.UUID{}, err
	}

	// We make a select transaction using the interface name
	// Then get the Port UUID from it
	portUUID := row["_uuid"].(ovsdb.UUID)

	return portUUID, nil
}

// IsMirrorConsumerAlreadyAttached Checks if the 'output_port' column of a mirror consumer contains a port UUID
func (ovsd *OvsDriver) IsMirrorConsumerAlreadyAttached(mirrorName string) (bool, error) {
	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, mirrorName)
	row, err := ovsd.findByCondition("Mirror", condition, nil)
	if err != nil {
		return false, err
	}

	outputPorts, err := convertToArray(row["output_port"])
	if err != nil {
		return false, fmt.Errorf("cannot convert output_port to an array error: %v", err)
	}

	if len(outputPorts) == 0 {
		return false, nil
	}
	return true, nil
}

// CheckMirrorProducerWithPorts Checks the configuration of a mirror producer based on ingress and egress values
func (ovsd *OvsDriver) CheckMirrorProducerWithPorts(mirrorName string, ingress, egress bool, portUUIDStr string) (bool, error) {
	portUUID := ovsdb.UUID{GoUUID: portUUIDStr}

	var conditions []ovsdb.Condition = []ovsdb.Condition{}
	conditionName := ovsdb.NewCondition("name", ovsdb.ConditionEqual, mirrorName)
	conditions = append(conditions, conditionName)
	if ingress {
		// select_src_port = Ports on which arriving packets are selected for mirroring
		conditionIngress := ovsdb.NewCondition("select_src_port", ovsdb.ConditionIncludes, portUUID)
		conditions = append(conditions, conditionIngress)
	}
	if egress {
		// select_dst_port = Ports on which departing packets are selected for mirroring
		conditionsEgress := ovsdb.NewCondition("select_dst_port", ovsdb.ConditionIncludes, portUUID)
		conditions = append(conditions, conditionsEgress)
	}

	// We cannot call findByCondition because we need to pass an array of conditions.
	// Also, there is no need to return an error if mirror doesn't exist, because in that case we want to create a new one
	return ovsd.isMirrorExistsByConditions(conditions)
}

// CheckMirrorConsumerWithPorts Checks the configuration of a mirror consumer
func (ovsd *OvsDriver) CheckMirrorConsumerWithPorts(mirrorName string, portUUIDStr string) (bool, error) {
	portUUID := ovsdb.UUID{GoUUID: portUUIDStr}

	var conditions []ovsdb.Condition = []ovsdb.Condition{}
	conditionName := ovsdb.NewCondition("name", ovsdb.ConditionEqual, mirrorName)
	conditions = append(conditions, conditionName)

	// output_port = Output port for selected packets
	conditionOutput := ovsdb.NewCondition("output_port", ovsdb.ConditionEqual, portUUID)
	conditions = append(conditions, conditionOutput)

	// We cannot call findByCondition because we need to pass an array of conditions.
	// Also, there is no need to return an error if mirror doesn't exist, because in that case we want to create a new one
	return ovsd.isMirrorExistsByConditions(conditions)
}

// IsMirrorPresent Checks if the Mirror entry already exists
func (ovsd *OvsDriver) IsMirrorPresent(mirrorName string) (bool, error) {
	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, mirrorName)
	selectOp := []ovsdb.Operation{{
		Op:      "select",
		Table:   "Mirror",
		Where:   []ovsdb.Condition{condition},
		Columns: []string{"name"},
	}}

	transactionResult, err := ovsd.ovsdbTransact(selectOp)
	if err != nil {
		return false, err
	}

	if len(transactionResult) != 1 {
		return false, fmt.Errorf("unknow error")
	}

	operationResult := transactionResult[0]
	if operationResult.Error != "" {
		return false, fmt.Errorf("%s - %s", operationResult.Error, operationResult.Details)
	}

	if len(operationResult.Rows) != 1 {
		return false, nil
	}

	return true, nil
}

// IsBridgePresent Checks if the bridge entry already exists
func (ovsd *OvsDriver) IsBridgePresent(bridgeName string) (bool, error) {
	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, bridgeName)
	selectOp := []ovsdb.Operation{{
		Op:      "select",
		Table:   "Bridge",
		Where:   []ovsdb.Condition{condition},
		Columns: []string{"name"},
	}}

	transactionResult, err := ovsd.ovsdbTransact(selectOp)
	if err != nil {
		return false, err
	}

	if len(transactionResult) != 1 {
		return false, fmt.Errorf("unknow error")
	}

	operationResult := transactionResult[0]
	if operationResult.Error != "" {
		return false, fmt.Errorf("%s - %s", operationResult.Error, operationResult.Details)
	}

	if len(operationResult.Rows) != 1 {
		return false, nil
	}

	return true, nil
}

// GetOvsPortForContIface Return ovs port name for an container interface
func (ovsd *OvsDriver) GetOvsPortForContIface(contIface, contNetnsPath string) (string, bool, error) {
	searchMap := map[string]string{
		"contNetns": contNetnsPath,
		"contIface": contIface,
		"owner":     ovsPortOwner,
	}
	ovsmap, err := ovsdb.NewOvsMap(searchMap)
	if err != nil {
		return "", false, err
	}

	condition := ovsdb.NewCondition("external_ids", ovsdb.ConditionEqual, ovsmap)
	colums := []string{"name", "external_ids"}
	port, err := ovsd.findByCondition("Port", condition, colums)
	if err != nil {
		return "", false, err
	}

	return fmt.Sprintf("%v", port["name"]), true, nil
}

// CleanEmptyMirrors removes all empty mirrors
func (ovsd *OvsBridgeDriver) CleanEmptyMirrors() error {
	mirrorNames, err := ovsd.findEmptyMirrors()
	if err != nil {
		return fmt.Errorf("clean mirrors: %v", err)
	}
	for _, mirrorName := range mirrorNames {
		log.Printf("Info: mirror %s is empty: removing it", mirrorName)
		if err := ovsd.DeleteMirror(ovsd.OvsBridgeName, mirrorName); err != nil {
			// Don't return an error here, just log its occurrence.
			// Something else may have removed the mirror already.
			log.Printf("cleanEmptyMirrors Error: %v\n", err)
		}
	}
	return nil
}

// FindInterfacesWithError returns the interfaces which are in error state
func (ovsd *OvsDriver) FindInterfacesWithError() ([]string, error) {
	selectOp := ovsdb.Operation{
		Op:      "select",
		Columns: []string{"name", "error"},
		Table:   "Interface",
	}
	transactionResult, err := ovsd.ovsdbTransact([]ovsdb.Operation{selectOp})
	if err != nil {
		return nil, err
	}
	if len(transactionResult) != 1 {
		return nil, fmt.Errorf("no transaction result")
	}
	operationResult := transactionResult[0]
	if operationResult.Error != "" {
		return nil, fmt.Errorf(operationResult.Error)
	}

	var names []string
	for _, row := range operationResult.Rows {
		if !hasError(row) {
			continue
		}
		names = append(names, fmt.Sprintf("%v", row["name"]))
	}
	if len(names) > 0 {
		log.Printf("found %d interfaces with error", len(names))
	}
	return names, nil
}

func hasError(row map[string]interface{}) bool {
	v := row["error"]
	switch x := v.(type) {
	case string:
		return x != ""
	default:
		return false
	}
}

// ************************ Notification handler for OVS DB changes ****************

// Update yet to be implemented
func (ovsd *OvsDriver) Update(context interface{}, tableUpdates ovsdb.TableUpdates) {
}

// Disconnected yet to be implemented
func (ovsd *OvsDriver) Disconnected(ovsClient client.Client) {
}

// Locked yet to be implemented
func (ovsd *OvsDriver) Locked([]interface{}) {
}

// Stolen yet to be implemented
func (ovsd *OvsDriver) Stolen([]interface{}) {
}

// Echo yet to be implemented
func (ovsd *OvsDriver) Echo([]interface{}) {
}

// ************************ Helper functions ********************
func (ovsd *OvsDriver) findByCondition(table string, condition ovsdb.Condition, columns []string) (map[string]interface{}, error) {
	selectOp := ovsdb.Operation{
		Op:    "select",
		Table: table,
		Where: []ovsdb.Condition{condition},
	}

	if columns != nil {
		selectOp.Columns = columns
	}

	transactionResult, err := ovsd.ovsdbTransact([]ovsdb.Operation{selectOp})
	if err != nil {
		return nil, err
	}

	if len(transactionResult) != 1 {
		return nil, fmt.Errorf("unknown error")
	}

	operationResult := transactionResult[0]
	if operationResult.Error != "" {
		return nil, fmt.Errorf("%s - %s", operationResult.Error, operationResult.Details)
	}

	if len(operationResult.Rows) != 1 {
		return nil, fmt.Errorf("failed to find object from table %s", table)
	}

	return operationResult.Rows[0], nil
}

// isMirrorExistsByConditions find a mirror by a list conditions.
// It returns true, only if there is a single row as result.
func (ovsd *OvsDriver) isMirrorExistsByConditions(conditions []ovsdb.Condition) (bool, error) {
	selectOp := []ovsdb.Operation{{
		Op:      "select",
		Table:   "Mirror",
		Where:   conditions,
		Columns: []string{"name"},
	}}

	transactionResult, err := ovsd.ovsdbTransact(selectOp)
	if err != nil {
		return false, err
	}

	if len(transactionResult) != 1 {
		return false, nil
	}

	operationResult := transactionResult[0]
	if operationResult.Error != "" {
		return false, fmt.Errorf("%s - %s", operationResult.Error, operationResult.Details)
	}

	if len(operationResult.Rows) != 1 {
		return false, nil
	}

	return true, nil
}

func createInterfaceOperation(intfName string, ofportRequest uint, ovnPortName string, intfType string) (ovsdb.UUID, *ovsdb.Operation, error) {
	intfUUIDStr := fmt.Sprintf("Intf%s", intfName)
	intfUUID := ovsdb.UUID{GoUUID: intfUUIDStr}

	intf := make(map[string]interface{})
	intf["name"] = intfName

	// Configure interface type if not nil
	if intfType != "" {
		intf["type"] = intfType
	}

	// Configure interface ID for ovn
	if ovnPortName != "" {
		oMap, err := ovsdb.NewOvsMap(map[string]string{"iface-id": ovnPortName})
		if err != nil {
			return ovsdb.UUID{}, nil, err
		}
		intf["external_ids"] = oMap
	}

	// Requested OpenFlow port number for this interface
	if ofportRequest != 0 {
		intf["ofport_request"] = ofportRequest
	}

	// Add an entry in Interface table
	intfOp := ovsdb.Operation{
		Op:       "insert",
		Table:    "Interface",
		Row:      intf,
		UUIDName: intfUUIDStr,
	}

	return intfUUID, &intfOp, nil
}

func createPortOperation(intfName, contNetnsPath, contIfaceName string, vlanTag uint, trunks []uint, portType string, intfUUID ovsdb.UUID) (ovsdb.UUID, *ovsdb.Operation, error) {
	portUUIDStr := intfName
	portUUID := ovsdb.UUID{GoUUID: portUUIDStr}

	port := make(map[string]interface{})
	port["name"] = intfName

	port["vlan_mode"] = portType
	var err error

	if portType != "trunk" && vlanTag != 0 {
		port["tag"] = vlanTag
	}

	if len(trunks) > 0 {
		port["trunks"], err = ovsdb.NewOvsSet(trunks)
		if err != nil {
			return ovsdb.UUID{}, nil, err
		}
	}

	port["interfaces"], err = ovsdb.NewOvsSet(intfUUID)
	if err != nil {
		return ovsdb.UUID{}, nil, err
	}

	oMap, err := ovsdb.NewOvsMap(map[string]string{
		"contNetns": contNetnsPath,
		"contIface": contIfaceName,
		"owner":     ovsPortOwner,
	})
	if err != nil {
		return ovsdb.UUID{}, nil, err
	}
	port["external_ids"] = oMap

	// Add an entry in Port table
	portOp := ovsdb.Operation{
		Op:       "insert",
		Table:    "Port",
		Row:      port,
		UUIDName: portUUIDStr,
	}

	return portUUID, &portOp, nil
}

func attachPortOperation(portUUID ovsdb.UUID, bridgeName string) *ovsdb.Operation {
	// mutate the Ports column of the row in the Bridge table
	mutateSet, _ := ovsdb.NewOvsSet(portUUID)
	mutation := ovsdb.NewMutation("ports", ovsdb.MutateOperationInsert, mutateSet)
	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, bridgeName)
	mutateOp := ovsdb.Operation{
		Op:        "mutate",
		Table:     "Bridge",
		Mutations: []ovsdb.Mutation{*mutation},
		Where:     []ovsdb.Condition{condition},
	}

	return &mutateOp
}

func deleteInterfaceOperation(intfName string) *ovsdb.Operation {
	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, intfName)
	intfOp := ovsdb.Operation{
		Op:    "delete",
		Table: "Interface",
		Where: []ovsdb.Condition{condition},
	}

	return &intfOp
}

func deletePortOperation(intfName string) *ovsdb.Operation {
	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, intfName)
	portOp := ovsdb.Operation{
		Op:    "delete",
		Table: "Port",
		Where: []ovsdb.Condition{condition},
	}

	return &portOp
}

func detachPortOperation(portUUID ovsdb.UUID, bridgeName string) *ovsdb.Operation {
	// mutate the Ports column of the row in the Bridge table
	mutateSet, _ := ovsdb.NewOvsSet(portUUID)
	mutation := ovsdb.NewMutation("ports", ovsdb.MutateOperationDelete, mutateSet)
	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, bridgeName)
	mutateOp := ovsdb.Operation{
		Op:        "mutate",
		Table:     "Bridge",
		Mutations: []ovsdb.Mutation{*mutation},
		Where:     []ovsdb.Condition{condition},
	}

	return &mutateOp
}

func createMirrorOperation(mirrorName string) (ovsdb.UUID, *ovsdb.Operation, error) {
	// Create an operation 'named-uuid' with a simple string as defined in RFC7047.
	// Spec states that 'uuid-name is only meaningful within the scope of a single transaction'.
	// So we use a simple constant string.
	mirrorUUIDStr := "newMirror"
	mirrorUUID := ovsdb.UUID{GoUUID: mirrorUUIDStr}

	mirror := make(map[string]interface{})
	mirror["name"] = mirrorName

	oMap, err := ovsdb.NewOvsMap(map[string]string{
		"owner": ovsPortOwner,
	})
	if err != nil {
		return ovsdb.UUID{}, nil, err
	}
	mirror["external_ids"] = oMap

	// Add an entry in Port table
	mirrorOp := ovsdb.Operation{
		Op:       "insert",
		Table:    "Mirror",
		Row:      mirror,
		UUIDName: mirrorUUIDStr,
	}

	return mirrorUUID, &mirrorOp, nil
}

func attachPortToMirrorProducerOperation(portUUID ovsdb.UUID, mirrorName string, ingress, egress bool) *ovsdb.Operation {
	// mutate the Ingress and Egress columns of the row in the Mirror table
	mutateSet, _ := ovsdb.NewOvsSet(portUUID)
	var mutations []ovsdb.Mutation = []ovsdb.Mutation{}
	if ingress {
		// select_src_port = Ports on which arriving packets are selected for mirroring
		mutationIngress := ovsdb.NewMutation("select_src_port", ovsdb.MutateOperationInsert, mutateSet)
		mutations = append(mutations, *mutationIngress)
	}
	if egress {
		// select_dst_port = Ports on which departing packets are selected for mirroring
		mutationEgress := ovsdb.NewMutation("select_dst_port", ovsdb.MutateOperationInsert, mutateSet)
		mutations = append(mutations, *mutationEgress)
	}

	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, mirrorName)
	mutateOp := ovsdb.Operation{
		Op:        "mutate",
		Table:     "Mirror",
		Mutations: mutations,
		Where:     []ovsdb.Condition{condition},
	}

	return &mutateOp
}

func attachPortToMirrorConsumerOperation(portUUID ovsdb.UUID, mirrorName string) *ovsdb.Operation {
	mutateSet, _ := ovsdb.NewOvsSet(portUUID)
	// output_port = Output port for selected packets
	mutation := ovsdb.NewMutation("output_port", ovsdb.MutateOperationInsert, mutateSet)

	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, mirrorName)
	mutateOp := ovsdb.Operation{
		Op:        "mutate",
		Table:     "Mirror",
		Mutations: []ovsdb.Mutation{*mutation},
		Where:     []ovsdb.Condition{condition},
	}

	return &mutateOp
}

func attachMirrorOperation(mirrorUUID ovsdb.UUID, bridgeName string) *ovsdb.Operation {
	// mutate the Mirrors column of the row in the Bridge table
	mutateSet, _ := ovsdb.NewOvsSet(mirrorUUID)
	mutation := ovsdb.NewMutation("mirrors", ovsdb.MutateOperationInsert, mutateSet)
	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, bridgeName)
	mutateOp := ovsdb.Operation{
		Op:        "mutate",
		Table:     "Bridge",
		Mutations: []ovsdb.Mutation{*mutation},
		Where:     []ovsdb.Condition{condition},
	}

	return &mutateOp
}

func detachPortFromMirrorOperation(portUUID ovsdb.UUID, mirrorName string, mirrorType int) *ovsdb.Operation {
	// mutate the Ports column of the row in the Bridge table
	var mutations []ovsdb.Mutation = []ovsdb.Mutation{}
	switch mirrorType {
	case MirrorProducer:
		mutateSet, _ := ovsdb.NewOvsSet(portUUID)
		// select_src_port = Ports on which arriving packets are selected for mirroring
		mutationIngress := ovsdb.NewMutation("select_src_port", ovsdb.MutateOperationDelete, mutateSet)
		// select_dst_port = Ports on which departing packets are selected for mirroring
		mutationEgress := ovsdb.NewMutation("select_dst_port", ovsdb.MutateOperationDelete, mutateSet)
		mutations = append(mutations, *mutationIngress, *mutationEgress)
	case MirrorConsumer:
		// output_port = Output port for selected packets
		mutationOutput := ovsdb.NewMutation("output_port", ovsdb.MutateOperationDelete, portUUID)
		mutations = append(mutations, *mutationOutput)
	default:
		log.Printf("skipping detatch mirror operation because mirrorType is unknown for mirror %s", mirrorName)
	}

	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, mirrorName)
	mutateOp := ovsdb.Operation{
		Op:        "mutate",
		Table:     "Mirror",
		Mutations: mutations,
		Where:     []ovsdb.Condition{condition},
	}

	return &mutateOp
}

func deleteMirrorOperation(mirrorName string) *ovsdb.Operation {
	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, mirrorName)
	mirrorOp := ovsdb.Operation{
		Op:    "delete",
		Table: "Mirror",
		Where: []ovsdb.Condition{condition},
	}

	return &mirrorOp
}

func detachMirrorFromBridgeOperation(mirrorUUID ovsdb.UUID, bridgeName string) *ovsdb.Operation {
	// mutate the Ports column of the row in the Bridge table
	mutateSet, _ := ovsdb.NewOvsSet(mirrorUUID)
	mutation := ovsdb.NewMutation("mirrors", ovsdb.MutateOperationDelete, mutateSet)
	condition := ovsdb.NewCondition("name", ovsdb.ConditionEqual, bridgeName)
	mutateOp := ovsdb.Operation{
		Op:        "mutate",
		Table:     "Bridge",
		Mutations: []ovsdb.Mutation{*mutation},
		Where:     []ovsdb.Condition{condition},
	}

	return &mutateOp
}

// findEmptyMirrors returns the empty mirrors (no select_src_port, select_dst_port and output ports)
func (ovsd *OvsDriver) findEmptyMirrors() ([]string, error) {
	var names []string

	// get all mirrors
	selectOp := ovsdb.Operation{
		Op:      "select",
		Columns: []string{"name", "output_port", "select_src_port", "select_dst_port"},
		Table:   "Mirror",
	}
	transactionResult, err := ovsd.ovsdbTransact([]ovsdb.Operation{selectOp})
	if err != nil {
		return nil, err
	}
	if len(transactionResult) != 1 {
		return nil, fmt.Errorf("no transaction result")
	}
	operationResult := transactionResult[0]
	if operationResult.Error != "" {
		return nil, fmt.Errorf(operationResult.Error)
	}

	// extract mirror names with both output_port, select_src_port and select_dst_port empty
	for _, row := range operationResult.Rows {
		isEmpty, err := isMirrorEmpty(row)
		if err != nil {
			return nil, fmt.Errorf("cannot convert select_src_port to an array error: %v", err)
		}
		if isEmpty {
			names = append(names, fmt.Sprintf("%v", row["name"]))
		}
	}

	if len(names) > 0 {
		log.Printf("found %d empty mirrors", len(names))
	}
	return names, nil
}

// isMirrorEmpty Checks if a mirror db row has both output_port, select_src_port and select_dst_port empty
func isMirrorEmpty(dbRow map[string]interface{}) (bool, error) {
	// Workaround to check output_port, select_dst_port and select_src_port consistenly, processing all
	// of them as array of UUIDs.
	// This is useful because ovn-org/libovsdb:
	// - when dbRow["column"] is empty in ovsdb, it returns an empty ovsdb.OvsSet
	// - when dbRow["column"] contains an UUID reference, it returns a ovsdb.UUID (not ovsdb.OvsSet)
	// - when dbRow["column"] contains multiple UUID references, it returns an ovsdb.OvsSet with the elements
	selectSrcPorts, err := convertToArray(dbRow["select_src_port"])
	if err != nil {
		return false, fmt.Errorf("cannot convert select_src_port to an array error: %v", err)
	}
	selectDstPorts, err := convertToArray(dbRow["select_dst_port"])
	if err != nil {
		return false, fmt.Errorf("cannot convert select_dst_port to an array error: %v", err)
	}
	outputPorts, err := convertToArray(dbRow["output_port"])
	if err != nil {
		return false, fmt.Errorf("cannot convert output_port to an array error: %v", err)
	}
	isEmpty := len(selectSrcPorts) == 0 && len(selectDstPorts) == 0 && len(outputPorts) == 0
	return isEmpty, nil
}

// utility function to convert an element (UUID or OvsSet) to an array of UUIDs
func convertToArray(elem interface{}) ([]interface{}, error) {
	elemType := reflect.TypeOf(elem)
	if elemType.Kind() == reflect.Struct {
		if elemType.Name() == "UUID" {
			return []interface{}{elem}, nil
		} else if elemType.Name() == "OvsSet" {
			return elem.(ovsdb.OvsSet).GoSet, nil
		}
		return nil, errors.New("struct with unknown types")
	}
	return nil, errors.New("unknown type")
}
