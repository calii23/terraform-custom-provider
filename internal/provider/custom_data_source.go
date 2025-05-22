package provider

import (
	"context"
	"fmt"
	"github.com/calii23/terraform-provider-custom/internal/connector"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var _ datasource.DataSource = &CustomDataSource{}
var _ datasource.DataSourceWithConfigure = &CustomDataSource{}

func NewCustomDataSource() datasource.DataSource {
	return &CustomDataSource{}
}

type CustomDataSourceModel struct {
	Name  string                 `tfsdk:"name"`
	Input basetypes.DynamicValue `tfsdk:"input"`
	State basetypes.DynamicValue `tfsdk:"state"`
}

type CustomDataSource struct {
	connectors map[string]connector.DataSourceConnector
}

func (c *CustomDataSource) Metadata(_ context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_data_source"
}

func (c *CustomDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "A custom data source that can be used to interact with custom scripts. See the provider documentation for details how to implement a custom script.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the script. This must match with the `name` attribute in the script.",
			},
			"input": schema.DynamicAttribute{
				Required:            true,
				MarkdownDescription: "The input for the custom data source. This is a map of strings to anything.",
			},
			"state": schema.DynamicAttribute{
				Computed:            true,
				MarkdownDescription: "The state of the custom data source. This is a map of strings to anything.",
			},
		},
	}
}

func (c *CustomDataSource) Configure(_ context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}

	connectors, ok := request.ProviderData.(*map[string]connector.DataSourceConnector)

	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *map[string]connector.DataSourceConnector, got: %T. Please report this issue to the provider developers.", request.ProviderData),
		)

		return
	}

	c.connectors = *connectors
}

func (c *CustomDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data CustomDataSourceModel

	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)

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

	state, connectorErr := resourceConnector.Read(ctx, input)
	if connectorErr != nil {
		response.Diagnostics.AddError(connectorErr.Summary, connectorErr.Detail)
		return
	}

	stateDynamic, err := mapToDynamic(ctx, state)
	if err != nil {
		response.Diagnostics.AddError("Failed to serialize state to dynamic", err.Error())
		return
	}

	data.State = stateDynamic
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}
