# FinOps Composition Definition Parser
This module is part of the FinOps Data Presentation component, in the Krateo Composable FinOps.

## Summary

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Configuration](#configuration)

## Overview
This module listens for events from the [eventrouter](https://github.com/krateoplatformops/eventrouter), and when it detects an `ExternalResourceCreated` on an object with apiVersion `core.krateo.io` and kind `CompositionDefinition`, it obtains the chart specified in the custom resource to look for `ANNOTATION_LABEL` annotations in the chart. These labels are then sent to the [finops-database-handler](https://github.com/krateoplatformops/finpos-database-handler) pricing notebook for storage. The labels are used by the frontend notebook to create an endpoint that, when called with the composition definition UID, returns the pricing of the resources in the composition definition. 
The [pricing_frontend](#frontend-presentation) notebook, allows the frontend to obtain a JSON that summarizes prices for the current composition definition. Given a composition definition uid as input, it will return the following:
```json
{"<unit of measure>": <value>}
```
For example:
```json
{"1 Hour": 7.2}
```

The pricing information can be placed in the database by creating a FocusConfig and specifying a dedicated table (i.e., the same used in the notebook) in the ScraperConfig. Otheriwse, it can be automated through the [operator generator](https://github.com/krateoplatformops/oasgen-provider). For example, on Azure you can use the [Azure Pricing Rest Dynamic Controller Plugin](https://github.com/krateoplatformops/azure-pricing-rest-dynamic-controller-plugin) and the [Focus Data Presentation Azure Composition](https://github.com/krateoplatformops/focus-data-presentation-azure).

## Architecture
In the diagram, this component is the `composition-definition-parser`.

![FinOps Composition Definition Parser](_diagrams/architecture.png)

## Configuration
This component requires the following notebook to be available at `URL_DATABASE_HANDLER_PRICING_NOTEBOOK` for the upload of the annotations to the database. The table used is specified in the notebook. By default, it will use `composition_definition_annotations`.

```python
# Note: the notebook is injected with additional lines of code by the finops-database-handler to setup the connection and cursor for the database
def main(operation : str, composition_id : str, json_list : str, table_name : str):
    try: 
        cursor.execute(f"CREATE TABLE IF NOT EXISTs {table_name} (composition_id string, keys object, PRIMARY KEY (composition_id)) WITH (column_policy = 'dynamic')")
    except Exception as e:
        print(f"Could not create table: {str(e)}")
    try:
        if operation == 'create':
            cursor.execute(f"INSERT INTO {table_name} (composition_id, keys) VALUES (?,?) ON CONFLICT (composition_id) DO UPDATE SET keys = excluded.keys;", [composition_id, json_list])
        else:
            cursor.execute(f"DELETE FROM {table_name} WHERE composition_id = '{composition_id}'")
    except Exception as e:
        print(f"Could not complete {operation} for {composition_id} in table {table_name}: {str(e)}")
    finally:
        cursor.close()

if __name__ == "__main__":
    args = {'operation': 'create', 'composition_id': '', 'json_list': '', 'annotation_table': 'composition_definition_annotations'}
    for i in range(5, len(sys.argv)):
        key_value = sys.argv[i]
        key_value_split = str.split(key_value, '=')
        if key_value_split[0] in args.keys():
            args[key_value_split[0]] = key_value_split[1] if key_value_split[1] else args[key_value_split[0]]

    for key in args:
        if args[key] == '':
            print('missing agument for call: ' + key)

    main(args['operation'], args['composition_id'], args['json_list'], args['annotation_table'])
``` 

### Configuring pricing
To upload pricing information to the database, you can create a FocusConfig from the [finops-operator-focus](https://github.com/krateoplatformops/finops-operator-focus), for example:
```yaml
apiVersion: finops.krateo.io/v1
kind: FocusConfig
metadata:
  name: {{ include "focus-data-presentation-azure.fullname" . }}
spec:
  scraperConfig:
    tableName: pricing_table
    pollingIntervalHours: 1
    scraperDatabaseConfigRef: 
      name: database-config
      namespace: finops
  focusSpec:
    availabilityZone: EU
    regionName: westeurope
    listUnitPrice: 0.45
    pricingUnit: KWh
    billedCost: 0.0 # do not modify
    billingCurrency: EUR
    billingPeriodEnd: "2024-01-01T00:00:00+02:00" # do not modify
    billingPeriodStart: "2024-01-01T00:00:00+02:00" # do not modify
    chargeCategory: "Usage" 
    chargeDescription: "Pricing information"
    chargePeriodEnd: "2024-01-01T00:00:00+02:00" # do not modify
    chargePeriodStart: "2024-01-01T00:00:00+02:00" # do not modify
    consumedQuantity: 0 # do not modify
    consumedUnit: KWh
    contractedCost: 0 # do not modify
    invoiceIssuerName: "Energy Provider"
    resourceName: "Energy"
    serviceCategory: "Energy"
    serviceName: "Energy"
    skuId: 0000
    tags:
    - key: "krateo-finops-focus-resource"
      value: "Energy"
```

This configuration will automatically start an export and a scraper to upload the data to the database. The table specified in the `ScraperConfig` should be the same used in frontend notebook. 

Otherwise, if there is a pricing API available, you can create a plugin for the Kubernetes Operator Generator and a composition definition to automate this process. For example, for Azure, you can use the [Focus Data Presentation Azure Composition](https://github.com/krateoplatformops/focus-data-presentation-azure) and simply create the following custom resource:
```yaml
apiVersion: composition.krateo.io/v0-1-0
kind: FocusDataPresentationAzure
metadata:
  name: sample
  namespace: krateo-system
spec:
  filter: serviceFamily eq 'Compute' and armRegionName eq 'westeurope' and skuId eq 'DZH318Z08NRP/001B' and type eq 'Consumption'
  scraperConfig:
    tableName: pricing_table
    pollingIntervalHours: 1
    scraperDatabaseConfigRef: 
      name: pricing_table
      namespace: krateo-system
```

### Frontend presentation notebook
The frontend presentation notebook allows the frontend to query the database for pricing information:
```python
# Note: the notebook is injected with additional lines of code by the finops-database-handler to setup the connection and cursor for the database
import json
def main(pricing_table : str, annotation_table : str, composition_id : str):
    try:
        cursor.execute(f"SELECT keys FROM {annotation_table} WHERE composition_id = '{composition_id}'")
        records = cursor.fetchall()
        result = {}
        for record in records:
            for row in record:
                for key in row.keys():
                    cursor.execute(f"SELECT listunitprice, pricingunit FROM {pricing_table} WHERE tags['krateo-finops-focus-resource'] = '{key}'")
                    inner_records = cursor.fetchall()
                    for inner_record in inner_records:
                        # 0: listunitprice, 1: pricingunit
                        if inner_record[1] in result.keys():
                            result[inner_record[1]] += float(inner_record[0])
                        else:
                            result[inner_record[1]] = float(inner_record[0])
        print(json.dumps(result))
                        
    except Exception as e:
        print(f"Could not insert keys into table: {str(e)}")
    finally:
        cursor.close()

if __name__ == "__main__":
    composition_id_key_value = sys.argv[5]
    composition_id_key_value_split = str.split(composition_id_key_value, '=')
    if composition_id_key_value_split[0] == 'composition_id':
        composition_id = composition_id_key_value_split[1]

    pricing_table_key_value = sys.argv[6]
    pricing_table_key_value_split = str.split(pricing_table_key_value, '=')
    if pricing_table_key_value_split[0] == 'pricing_table':
        pricing_table = pricing_table_key_value_split[1]

    annotation_table_key_value = sys.argv[7]
    annotation_table_key_value_split = str.split(annotation_table_key_value, '=')
    if annotation_table_key_value_split[0] == 'annotation_table':
        annotation_table = annotation_table_key_value_split[1]

    if pricing_table == '':
        pricing_table = 'pricing_table'
    if annotation_table == '':
        annotation_table = 'composition_definition_annotations'

    main(pricing_table, annotation_table, composition_id)
```