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
	"errors"
	"fmt"

	"github.com/socketplane/libovsdb"
)

// OVS driver state
type OvsDriver struct {
	// OVS client
	ovsClient *libovsdb.OvsdbClient
}

type OvsBridgeDriver struct {
	OvsDriver

	// Name of the OVS bridge
	OvsBridgeName string
}

// Create a new OVS driver with Unix socket
func NewOvsDriver(ovsSocket string) (*OvsDriver, error) {
	ovsDriver := new(OvsDriver)

	ovsDB, err := libovsdb.ConnectWithUnixSocket(ovsSocket)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ovsdb error: %v", err)
	}

	// Setup state
	ovsDriver.ovsClient = ovsDB

	return ovsDriver, nil
}

// Create a new OVS driver for a bridge with Unix socket
func NewOvsBridgeDriver(bridgeName string) (*OvsBridgeDriver, error) {
	ovsDriver := new(OvsBridgeDriver)

	ovsDB, err := libovsdb.ConnectWithUnixSocket("/var/run/openvswitch/db.sock")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ovsdb error: %v", err)
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
func (self *OvsDriver) ovsdbTransact(ops []libovsdb.Operation) ([]libovsdb.OperationResult, error) {
	// Perform OVSDB transaction
	reply, _ := self.ovsClient.Transact("Open_vSwitch", ops...)

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
// Create an internal port in OVS
func (self *OvsBridgeDriver) CreatePort(intfName, contNetnsPath, contIfaceName, ovnPortName string, vlanTag uint) error {
	intfUuid, intfOp, err := createInterfaceOperation(intfName, ovnPortName)
	if err != nil {
		return err
	}

	portUuid, portOp, err := createPortOperation(intfName, contNetnsPath, contIfaceName, vlanTag, intfUuid)
	if err != nil {
		return err
	}

	mutateOp := attachPortOperation(portUuid, self.OvsBridgeName)

	// Perform OVS transaction
	operations := []libovsdb.Operation{*intfOp, *portOp, *mutateOp}

	_, err = self.ovsdbTransact(operations)
	return err
}

// Delete a port from OVS
func (self *OvsBridgeDriver) DeletePort(intfName string) error {
	condition := libovsdb.NewCondition("name", "==", intfName)
	row, err := self.findByCondition("Port", condition, nil)
	if err != nil {
		return err
	}

	// We make a select transaction using the interface name
	// Then get the Port UUID from it
	portUuidStr := row["_uuid"].([]interface{})
	portUuid := []libovsdb.UUID{{GoUUID: fmt.Sprintf("%v", portUuidStr[1])}}

	intfOp := deleteInterfaceOperation(intfName)

	portOp := deletePortOperation(intfName)

	mutateOp := detachPortOperation(portUuid, self.OvsBridgeName)

	// Perform OVS transaction
	operations := []libovsdb.Operation{*intfOp, *portOp, *mutateOp}

	_, err = self.ovsdbTransact(operations)
	return err
}

func (self *OvsDriver) BridgeList() ([]string, error) {
	selectOp := []libovsdb.Operation{{
		Op:      "select",
		Table:   "Bridge",
		Columns: []string{"name"},
	}}

	transactionResult, err := self.ovsdbTransact(selectOp)
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

// Check if the bridge entry already exists
func (self *OvsDriver) IsBridgePresent(bridgeName string) (bool, error) {
	condition := libovsdb.NewCondition("name", "==", bridgeName)
	selectOp := []libovsdb.Operation{{
		Op:      "select",
		Table:   "Bridge",
		Where:   []interface{}{condition},
		Columns: []string{"name"},
	}}

	transactionResult, err := self.ovsdbTransact(selectOp)
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

// Return ovs port name for an container interface
func (self *OvsDriver) GetOvsPortForContIface(contIface, contNetnsPath string) (string, bool, error) {
	searchMap := map[string]string{"contNetns": contNetnsPath, "contIface": contIface}
	ovsmap, err := libovsdb.NewOvsMap(searchMap)
	if err != nil {
		return "", false, err
	}

	condition := libovsdb.NewCondition("external_ids", "==", ovsmap)
	colums := []string{"name", "external_ids"}
	port, err := self.findByCondition("Port", condition, colums)
	if err != nil {
		return "", false, err
	}

	return fmt.Sprintf("%v", port["name"]), true, nil
}

// ************************ Notification handler for OVS DB changes ****************
func (self *OvsDriver) Update(context interface{}, tableUpdates libovsdb.TableUpdates) {
}
func (self *OvsDriver) Disconnected(ovsClient *libovsdb.OvsdbClient) {
}
func (self *OvsDriver) Locked([]interface{}) {
}
func (self *OvsDriver) Stolen([]interface{}) {
}
func (self *OvsDriver) Echo([]interface{}) {
}

// ************************ Helper functions ********************
func (self *OvsDriver) findByCondition(table string, condition []interface{}, columns []string) (map[string]interface{}, error) {
	selectOp := libovsdb.Operation{
		Op:    "select",
		Table: table,
	}

	if condition != nil {
		selectOp.Where = []interface{}{condition}
	}
	if columns != nil {
		selectOp.Columns = columns
	}

	transactionResult, err := self.ovsdbTransact([]libovsdb.Operation{selectOp})
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

func createInterfaceOperation(intfName, ovnPortName string) ([]libovsdb.UUID, *libovsdb.Operation, error) {
	intfUuidStr := fmt.Sprintf("Intf%s", intfName)
	intfUuid := []libovsdb.UUID{{GoUUID: intfUuidStr}}

	intf := make(map[string]interface{})
	intf["name"] = intfName

	// Configure interface ID for ovn
	if ovnPortName != "" {
		oMap, err := libovsdb.NewOvsMap(map[string]string{"iface-id": ovnPortName})
		if err != nil {
			return nil, nil, err
		}
		intf["external_ids"] = oMap
	}

	// Add an entry in Interface table
	intfOp := libovsdb.Operation{
		Op:       "insert",
		Table:    "Interface",
		Row:      intf,
		UUIDName: intfUuidStr,
	}

	return intfUuid, &intfOp, nil
}

func createPortOperation(intfName, contNetnsPath, contIfaceName string, vlanTag uint, intfUuid []libovsdb.UUID) ([]libovsdb.UUID, *libovsdb.Operation, error) {
	portUuidStr := intfName
	portUuid := []libovsdb.UUID{{GoUUID: portUuidStr}}

	port := make(map[string]interface{})
	port["name"] = intfName
	if vlanTag != 0 {
		port["vlan_mode"] = "access"
		port["tag"] = vlanTag
	} else {
		port["vlan_mode"] = "trunk"
	}

	var err error
	port["interfaces"], err = libovsdb.NewOvsSet(intfUuid)
	if err != nil {
		return nil, nil, err
	}

	oMap, err := libovsdb.NewOvsMap(map[string]string{"contNetns": contNetnsPath, "contIface": contIfaceName})
	if err != nil {
		return nil, nil, err
	}
	port["external_ids"] = oMap

	// Add an entry in Port table
	portOp := libovsdb.Operation{
		Op:       "insert",
		Table:    "Port",
		Row:      port,
		UUIDName: portUuidStr,
	}

	return portUuid, &portOp, nil
}

func attachPortOperation(portUuid []libovsdb.UUID, bridgeName string) *libovsdb.Operation {
	// mutate the Ports column of the row in the Bridge table
	mutateSet, _ := libovsdb.NewOvsSet(portUuid)
	mutation := libovsdb.NewMutation("ports", "insert", mutateSet)
	condition := libovsdb.NewCondition("name", "==", bridgeName)
	mutateOp := libovsdb.Operation{
		Op:        "mutate",
		Table:     "Bridge",
		Mutations: []interface{}{mutation},
		Where:     []interface{}{condition},
	}

	return &mutateOp
}

func deleteInterfaceOperation(intfName string) *libovsdb.Operation {
	condition := libovsdb.NewCondition("name", "==", intfName)
	intfOp := libovsdb.Operation{
		Op:    "delete",
		Table: "Interface",
		Where: []interface{}{condition},
	}

	return &intfOp
}

func deletePortOperation(intfName string) *libovsdb.Operation {
	condition := libovsdb.NewCondition("name", "==", intfName)
	portOp := libovsdb.Operation{
		Op:    "delete",
		Table: "Port",
		Where: []interface{}{condition},
	}

	return &portOp
}

func detachPortOperation(portUuid []libovsdb.UUID, bridgeName string) *libovsdb.Operation {
	// mutate the Ports column of the row in the Bridge table
	mutateSet, _ := libovsdb.NewOvsSet(portUuid)
	mutation := libovsdb.NewMutation("ports", "delete", mutateSet)
	condition := libovsdb.NewCondition("name", "==", bridgeName)
	mutateOp := libovsdb.Operation{
		Op:        "mutate",
		Table:     "Bridge",
		Mutations: []interface{}{mutation},
		Where:     []interface{}{condition},
	}

	return &mutateOp
}
