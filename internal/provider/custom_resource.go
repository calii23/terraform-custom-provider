package provider

import (
	"context"
	"fmt"
	"github.com/calii23/terraform-provider-custom/internal/connector"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"math/big"
	"strings"
)

var _ resource.Resource = &CustomResource{}
var _ resource.ResourceWithModifyPlan = &CustomResource{}
var _ resource.ResourceWithConfigure = &CustomResource{}
var _ resource.ResourceWithImportState = &CustomResource{}

func NewCustomResource() resource.Resource {
	return &CustomResource{}
}

type CustomResource struct {
	connectors map[string]connector.ResourceConnector
}

type CustomResourceModel struct {
	Name  string                 `tfsdk:"name"`
	Input basetypes.DynamicValue `tfsdk:"input"`
	State basetypes.DynamicValue `tfsdk:"state"`
}

func (c *CustomResource) Metadata(_ context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_resource"
}

func (c *CustomResource) Schema(_ context.Context, _ resource.SchemaRequest, response *resource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "A custom resource that can be used to interact with custom scripts. See the provider documentation for details how to implement a custom script.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				MarkdownDescription: "The name of the script. This must match with the `name` attribute in the script.",
			},
			"input": schema.DynamicAttribute{
				Required:            true,
				MarkdownDescription: "The input for the custom resource. This is a map of strings to anything.",
			},
			"state": schema.DynamicAttribute{
				Computed:            true,
				MarkdownDescription: "The state of the custom resource. This is a map of strings to anything.",
			},
		},
	}
}

func (c *CustomResource) Configure(_ context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}

	connectors, ok := request.ProviderData.(*map[string]connector.ResourceConnector)

	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *map[string]connector.ResourceConnector, got: %T. Please report this issue to the provider developers.", request.ProviderData),
		)

		return
	}

	c.connectors = *connectors
}

func (c *CustomResource) ModifyPlan(ctx context.Context, request resource.ModifyPlanRequest, response *resource.ModifyPlanResponse) {
	var connectorName *string

	response.Diagnostics.Append(request.Plan.GetAttribute(ctx, path.Root("name"), &connectorName)...)

	if response.Diagnostics.HasError() {
		return
	}

	if connectorName == nil {
		return
	}

	resourceConnector, ok := c.connectors[*connectorName]
	if !ok {
		response.Diagnostics.AddError(
			"Invalid Connector Name",
			"Connector type not found: "+*connectorName,
		)
		return
	}

	for _, propertyName := range resourceConnector.RequiresReplacePaths() {
		response.RequiresReplace.Append(path.Root("input").AtMapKey(propertyName))
	}
}

func (c *CustomResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	var data CustomResourceModel

	response.Diagnostics.Append(request.Plan.Get(ctx, &data)...)

	if response.Diagnostics.HasError() {
		return
	}

	resourceConnector, ok := c.connectors[data.Name]
	if !ok {
		response.Diagnostics.AddError(
			"Invalid Connector Name",
			"Connector type not found: "+data.Name,
		)
		return
	}

	input, err := dynamicToMap(ctx, data.Input)
	if err != nil {
		response.Diagnostics.AddError("Failed to serialize input to map", err.Error())
		return
	}

	state, connectorErr := resourceConnector.Create(ctx, input)
	if connectorErr != nil {
		response.Diagnostics.AddError(connectorErr.Summary, connectorErr.Detail)
		return
	}

	stateDynamic, err := mapToDynamic(ctx, state)
	if err != nil {
		response.Diagnostics.AddError("Failed to convert map to dynamic value", err.Error())
		return
	}

	data.State = stateDynamic
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}

func (c *CustomResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var data CustomResourceModel

	response.Diagnostics.Append(request.State.Get(ctx, &data)...)

	if response.Diagnostics.HasError() {
		return
	}

	resourceConnector, ok := c.connectors[data.Name]
	if !ok {
		response.Diagnostics.AddError(
			"Invalid Connector Name",
			"Connector type not found: "+data.Name,
		)
		return
	}

	input, err := dynamicToMap(ctx, data.Input)
	if err != nil {
		response.Diagnostics.AddError("Failed to serialize input to map", err.Error())
		return
	}

	state, err := dynamicToMap(ctx, data.State)
	if err != nil {
		response.Diagnostics.AddError("Failed to serialize state to map", err.Error())
		return
	}

	updatedInput, updatedState, connectorErr := resourceConnector.Read(ctx, input, state)
	if connectorErr != nil {
		response.Diagnostics.AddError(connectorErr.Summary, connectorErr.Detail)
		return
	}

	updatedInputDynamic, err := mapToDynamic(ctx, updatedInput)
	if err != nil {
		response.Diagnostics.AddError("Failed to convert map to dynamic value", err.Error())
		return
	}
	data.Input = updatedInputDynamic

	updatedStateDynamic, err := mapToDynamic(ctx, updatedState)
	if err != nil {
		response.Diagnostics.AddError("Failed to convert map to dynamic value", err.Error())
		return
	}

	data.State = updatedStateDynamic
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}

func (c *CustomResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	var data CustomResourceModel

	response.Diagnostics.Append(request.State.Get(ctx, &data)...)

	if response.Diagnostics.HasError() {
		return
	}

	oldInput, err := dynamicToMap(ctx, data.Input)
	if err != nil {
		response.Diagnostics.AddError("Failed to serialize old input to map", err.Error())
		return
	}

	state, err := dynamicToMap(ctx, data.State)
	if err != nil {
		response.Diagnostics.AddError("Failed to serialize state to map", err.Error())
		return
	}

	response.Diagnostics.Append(request.Plan.Get(ctx, &data)...)

	if response.Diagnostics.HasError() {
		return
	}

	newInput, err := dynamicToMap(ctx, data.Input)
	if err != nil {
		response.Diagnostics.AddError("Failed to serialize new input to map", err.Error())
		return
	}

	resourceConnector, ok := c.connectors[data.Name]
	if !ok {
		response.Diagnostics.AddError(
			"Invalid Connector Name",
			"Connector type not found: "+data.Name,
		)
		return
	}

	updatedState, connectorErr := resourceConnector.Update(ctx, oldInput, newInput, state)
	if connectorErr != nil {
		response.Diagnostics.AddError(connectorErr.Summary, connectorErr.Detail)
		return
	}

	updatedStateDynamic, err := mapToDynamic(ctx, updatedState)
	if err != nil {
		response.Diagnostics.AddError("Failed to convert map to dynamic value", err.Error())
		return
	}

	data.State = updatedStateDynamic
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}

func (c *CustomResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	var data CustomResourceModel

	response.Diagnostics.Append(request.State.Get(ctx, &data)...)

	if response.Diagnostics.HasError() {
		return
	}

	resourceConnector, ok := c.connectors[data.Name]
	if !ok {
		response.Diagnostics.AddError(
			"Invalid Connector Name",
			"Connector type not found: "+data.Name,
		)
		return
	}

	input, err := dynamicToMap(ctx, data.Input)
	if err != nil {
		response.Diagnostics.AddError("Failed to serialize input to map", err.Error())
		return
	}

	state, err := dynamicToMap(ctx, data.State)
	if err != nil {
		response.Diagnostics.AddError("Failed to serialize state to map", err.Error())
		return
	}

	connectorErr := resourceConnector.Delete(ctx, input, state)
	if connectorErr != nil {
		response.Diagnostics.AddError(connectorErr.Summary, connectorErr.Detail)
		return
	}
}

func (c *CustomResource) ImportState(ctx context.Context, request resource.ImportStateRequest, response *resource.ImportStateResponse) {
	// Parse the ID - format is "type:id"
	id := request.ID

	colonIndex := strings.IndexByte(id, ':')

	if colonIndex == -1 {
		response.Diagnostics.AddError(
			"Invalid Import ID Format",
			"The import ID must be in the format 'type:id', where 'type' is the connector type.",
		)
		return
	}

	// Extract type and connector-specific ID
	connectorType := id[:colonIndex]
	connectorID := id[colonIndex+1:]

	// Find the connector
	resourceConnector, ok := c.connectors[connectorType]
	if !ok {
		response.Diagnostics.AddError(
			"Invalid Connector Name",
			"Connector type not found: "+connectorType,
		)
		return
	}

	// Call the connector's ImportState method
	input, state, connectorErr := resourceConnector.ImportState(ctx, connectorID)
	if connectorErr != nil {
		response.Diagnostics.AddError(connectorErr.Summary, connectorErr.Detail)
		return
	}

	// Convert the input and state to dynamic values
	inputDynamic, err := mapToDynamic(ctx, input)
	if err != nil {
		response.Diagnostics.AddError("Failed to convert input to dynamic value", err.Error())
		return
	}

	stateDynamic, err := mapToDynamic(ctx, state)
	if err != nil {
		response.Diagnostics.AddError("Failed to convert state to dynamic value", err.Error())
		return
	}

	// Create a new resource model with the imported data
	data := CustomResourceModel{
		Name:  connectorType,
		Input: inputDynamic,
		State: stateDynamic,
	}

	// Set the state
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}

func dynamicToMap(ctx context.Context, value basetypes.DynamicValue) (map[string]interface{}, error) {
	tfvalue, err := value.ToTerraformValue(ctx)
	if err != nil {
		return nil, err
	}

	outputValue, ok := unserializeValue(tfvalue).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to convert dynamic value to map")
	}

	return outputValue, nil
}

func mapToDynamic(ctx context.Context, value interface{}) (basetypes.DynamicValue, error) {
	if value == nil {
		return basetypes.NewDynamicNull(), nil
	}

	attrValue, err := serializeValue(ctx, value)
	if err != nil {
		return basetypes.DynamicValue{}, err
	}

	dynamicValue := basetypes.NewDynamicValue(attrValue)

	return dynamicValue, nil
}

func unserializeValue(value tftypes.Value) interface{} {
	if value.IsNull() {
		return nil
	}

	typ := value.Type()
	switch {
	case typ.Is(tftypes.String):
		var s string
		err := value.As(&s)
		if err != nil {
			panic(err)
		}
		return s
	case typ.Is(tftypes.Number):
		n := big.NewFloat(0)
		err := value.As(&n)
		if err != nil {
			panic(err)
		}

		if n.IsInt() {
			v, _ := n.Int64()
			return v
		} else {
			v, _ := n.Float64()
			return v
		}
	case typ.Is(tftypes.Bool):
		var b bool
		err := value.As(&b)
		if err != nil {
			panic(err)
		}
		return b
	case typ.Is(tftypes.List{}), typ.Is(tftypes.Set{}), typ.Is(tftypes.Tuple{}):
		var l []tftypes.Value
		err := value.As(&l)
		if err != nil {
			panic(err)
		}

		output := make([]interface{}, 0, len(l))
		for _, v := range l {
			output = append(output, unserializeValue(v))
		}

		return output
	case typ.Is(tftypes.Map{}), typ.Is(tftypes.Object{}):
		m := map[string]tftypes.Value{}
		err := value.As(&m)
		if err != nil {
			panic(err)
		}

		var output = map[string]interface{}{}
		for k, v := range m {
			output[k] = unserializeValue(v)
		}

		return output
	}

	panic(fmt.Sprintf("Unsupported type: %s", typ))
}

func serializeValue(ctx context.Context, value interface{}) (attr.Value, error) {
	if value == nil {
		return types.DynamicNull(), nil
	}

	switch v := value.(type) {
	case bool:
		return types.BoolValue(v), nil
	case float32:
		return types.Float32Value(v), nil
	case float64:
		return types.Float64Value(v), nil
	case int32:
		return types.Int32Value(v), nil
	case int64:
		return types.Int64Value(v), nil
	case string:
		return types.StringValue(v), nil
	case *big.Float:
		return types.NumberValue(v), nil
	case []interface{}:
		var elementTypes []attr.Type
		var elements []attr.Value

		for _, item := range v {
			elementValue, err := serializeValue(ctx, item)
			if err != nil {
				return nil, err
			}

			elementTypes = append(elementTypes, elementValue.Type(ctx))
			elements = append(elements, elementValue)
		}

		tupleValue, _ := types.TupleValue(elementTypes, elements)
		return tupleValue, nil
	case map[string]interface{}:
		attributeTypes := make(map[string]attr.Type)
		attributes := make(map[string]attr.Value)

		for k, item := range v {
			elementValue, err := serializeValue(ctx, item)
			if err != nil {
				return nil, err
			}

			attributeTypes[k] = elementValue.Type(ctx)
			attributes[k] = elementValue
		}

		objectValue, _ := types.ObjectValue(attributeTypes, attributes)
		return objectValue, nil
	case *bool:
	case *float32:
	case *float64:
	case *int32:
	case *int64:
	case *string:
	case *[]interface{}:
	case *map[string]interface{}:
		if v == nil {
			return types.DynamicNull(), nil
		}

		return serializeValue(ctx, *v)
	}

	return nil, fmt.Errorf("unsupported type: %T", value)
}
