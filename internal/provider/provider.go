package provider

import (
	"context"
	"github.com/calii23/terraform-provider-custom/internal/connector"
	"github.com/calii23/terraform-provider-custom/internal/connector/nodejs"
	"github.com/calii23/terraform-provider-custom/internal/customvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"time"
)

// Ensure CustomProvider satisfies various provider interfaces.
var _ provider.Provider = &CustomProvider{}

// CustomProvider defines the provider implementation.
type CustomProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

type CustomProviderModel struct {
	NodejsResources   []CustomNodejsScriptModel `tfsdk:"nodejs_resource"`
	NodejsDataSources []CustomNodejsScriptModel `tfsdk:"nodejs_data_source"`
}

type EnvironmentVariableModel struct {
	Name  string `tfsdk:"name"`
	Value string `tfsdk:"value"`
}

type CustomNodejsScriptModel struct {
	Path              string                     `tfsdk:"path"`
	Executable        basetypes.StringValue      `tfsdk:"executable"`
	Args              []string                   `tfsdk:"args"`
	InitializeTimeout basetypes.StringValue      `tfsdk:"initialize_timeout"`
	OperationTimeout  basetypes.StringValue      `tfsdk:"operation_timeout"`
	LogFile           basetypes.StringValue      `tfsdk:"log_file"`
	Environment       []EnvironmentVariableModel `tfsdk:"environment_variable"`
	InputType         basetypes.StringValue      `tfsdk:"input_type"`
}

func (m *CustomNodejsScriptModel) getNodejsExecutable() string {
	if m.Executable.IsNull() {
		return "node"
	}
	return m.Executable.ValueString()
}

func (m *CustomNodejsScriptModel) getInitializeTimeout() time.Duration {
	if m.InitializeTimeout.IsNull() {
		return 5 * time.Second
	}
	timeout, _ := time.ParseDuration(m.InitializeTimeout.ValueString())
	return timeout
}

func (m *CustomNodejsScriptModel) getOperationTimeout() time.Duration {
	if m.OperationTimeout.IsNull() {
		return 30 * time.Second
	}
	timeout, _ := time.ParseDuration(m.OperationTimeout.ValueString())
	return timeout
}

func (m *CustomNodejsScriptModel) getLogFile() *string {
	if m.LogFile.IsNull() {
		return nil
	}
	logFileValue := m.LogFile.ValueString()
	return &logFileValue
}

func (m *CustomNodejsScriptModel) getEnvironmentVariables() map[string]string {
	env := make(map[string]string)
	for _, envVar := range m.Environment {
		env[envVar.Name] = envVar.Value
	}
	return env
}

func (m *CustomNodejsScriptModel) getInputType() nodejs.InputType {
	if m.InputType.IsNull() {
		return nodejs.InputTypeAuto
	}
	switch m.InputType.ValueString() {
	case "module":
		return nodejs.InputTypeModule
	case "commonjs":
		return nodejs.InputTypeCommonJS
	default:
		return nodejs.InputTypeAuto
	}
}

func (p *CustomProvider) Metadata(_ context.Context, _ provider.MetadataRequest, response *provider.MetadataResponse) {
	response.TypeName = "custom"
	response.Version = p.version
}

func (p *CustomProvider) Schema(_ context.Context, _ provider.SchemaRequest, response *provider.SchemaResponse) {
	nodejsScriptBlock := schema.NestedBlockObject{
		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The path to the Node.js resource.",
			},
			"executable": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The path to the Node.js executable. If not set, the provider will use the default Node.js executable in the system PATH.",
			},
			"args": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "The arguments to pass to the Node.js executable. This is a list of strings.",
			},
			"initialize_timeout": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The timeout for the Node.js resource to initialize. This is a number of milliseconds. Defaults to 5 seconds.",
				Validators:          []validator.String{customvalidator.ValidDuration()},
			},
			"operation_timeout": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The timeout for the Node.js resource to perform an operation. This is a number of milliseconds. Defaults to 30 seconds.",
				Validators:          []validator.String{customvalidator.ValidDuration()},
			},
			"log_file": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The path to the log file for the Node.js resource. If not set, the provider will not log to a file.",
			},
			"input_type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The input type for the Node.js resource. This can be either `auto` (default), `module` or `commonjs`. If set to `auto` the script must have either `.cjs` or `.mjs` file extension.",
				Validators:          []validator.String{stringvalidator.OneOf("auto", "module", "commonjs")},
			},
		},
		Blocks: map[string]schema.Block{
			"environment_variable": schema.SetNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The name of the environment variable.",
						},
						"value": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The value of the environment variable.",
						},
					},
				},
				MarkdownDescription: "An environment variable to set for the Node.js resource.",
			},
		},
	}
	response.Schema = schema.Schema{
		Blocks: map[string]schema.Block{
			"nodejs_resource": schema.SetNestedBlock{
				NestedObject: nodejsScriptBlock,
				MarkdownDescription: `The ` + "`nodejs_resource`" + ` block allows you to implement custom Terraform resources using JavaScript/TypeScript through Node.js. This approach provides flexibility to create resources for services or APIs that don't have dedicated providers.

#### Resource Concepts

##### Input and State

The resource implementation is built around two core concepts:
- **Input**: The configuration provided by the user in the Terraform configuration. This represents what the user wants the resource to be.
- **State**: The actual state of the resource as it exists in the external system. This is what your script reports back after each operation.

The provider handles the mapping between Terraform's resource attributes and these objects, allowing your script to focus on the implementation logic.

##### Resource Lifecycle

A Node.js resource goes through the following lifecycle:

1. **Initialization**: The script is loaded and must return an object implementing the ` + "`CustomResource`" + ` interface.
2. **Validation**: Before any operation, ` + "`validateInput()`" + ` is called to verify the input is valid.
3. **Create**: When a new resource is being created, ` + "`create(input)`" + ` is called. It should create the resource in the external system and return its state.
4. **Read**: Called during refresh or after create/update operations via ` + "`read(input, state)`" + `. It should retrieve the current state from the external system.
5. **Update**: When a resource is modified, ` + "`update(oldInput, newInput, state)`" + ` is called. It should update the resource in the external system.
6. **Delete**: When a resource is being destroyed, ` + "`delete(input, state)`" + ` is called. It should remove the resource from the external system.
7. **Import**: When an existing resource is being imported, ` + "`import(id)`" + ` is called. It should retrieve both the input and state for an existing resource.

##### Name

The ` + "`name`" + ` attribute is a required string that identifies the resource. It is used to reference the resource in the Terraform configuration and must be unique within the provider.

#### TypeScript

In case you are using TypeScript, you can add the following types to your project. Keep in mind that you still need to transpile your TypeScript code to JavaScript before running it with the provider.

` + "```" + `typescript
declare global {
  class TerraformError extends Error {
    readonly summary: string;
    readonly detail: string;

    constructor(summary: string, detail: string);
  }
}

export interface CustomDataSource<I extends object, S extends object> {
  readonly name: string;

  validateInput(input: unknown): void;

  read(input: I): Promise<S>;
}

export interface CustomResource<I extends object, S extends object> {
  readonly name: string;

  validateInput(input: unknown): void;

  read(input: I, state: S): Promise<{ input: S, state: I }>;

  create(input: I): Promise<S>;

  update(oldInput: I, newInput: I, state: S): Promise<S>;

  delete(input: I, state: S): Promise<void>;

  import(id: string): Promise<{ input: I, state: S }>;
}
` + "```" + `

#### Example Implementation

Here is an example implementation of a custom resource using TypeScript. This example demonstrates how to create, read, update, and delete a resource using an external API.

` + "```" + `typescript
import axios from 'axios';

interface ResourceInput {
  name: string;
  description: string;
  tags: Record<string, string>;
}

interface ResourceState {
  id: string;
  name: string;
  description: string;
  tags: Record<string, string>;
  createdAt: string;
  lastUpdated: string;
}

const apiClient = axios.create({
  baseURL: 'https://api.example.com/v1',
  timeout: 5000,
  headers: {
    'Authorization': ` + "`Bearer ${process.env.API_TOKEN}`" + `,
    'Content-Type': 'application/json'
  }
});

const resource: CustomResource<ResourceInput, ResourceState> = {
  name: "example_resource",

  validateInput(input: unknown): void {
    const typedInput = input as ResourceInput;
    if (!typedInput.name) {
      throw new TerraformError(
        "Invalid input", 
        "The 'name' attribute is required"
      );
    }
    
    if (typedInput.name.length > 64) {
      throw new TerraformError(
        "Invalid name", 
        "Resource name cannot exceed 64 characters"
      );
    }
  },

  async create(input: ResourceInput): Promise<ResourceState> {
    try {
      const response = await apiClient.post('/resources', input);
      return response.data;
    } catch (error) {
      throw new TerraformError(
        "Failed to create resource",
        error.message
      );
    }
  },

  async read(input: ResourceInput, state: ResourceState): Promise<{ input: ResourceInput, state: ResourceState }> {
    try {
      const response = await apiClient.get(` + "`/resources/${state.id}`" + `);
      
      // If resource not found, return null to indicate it's been deleted
      if (response.status === 404) {
        return null;
      }
      
      return {
        input,
        state: response.data
      };
    } catch (error) {
      throw new TerraformError(
        "Failed to read resource",
        error.message
      );
    }
  },

  async update(oldInput: ResourceInput, newInput: ResourceInput, state: ResourceState): Promise<ResourceState> {
    try {
      const response = await apiClient.put(` + "`/resources/${state.id}`" + `, newInput);
      return response.data;
    } catch (error) {
      throw new TerraformError(
        "Failed to update resource",
        error.message
      );
    }
  },

  async delete(input: ResourceInput, state: ResourceState): Promise<void> {
    try {
      await apiClient.delete(` + "`/resources/${state.id}`" + `);
    } catch (error) {
      // If resource not found (404), consider it already deleted
      if (error.response && error.response.status === 404) {
        return;
      }
      
      throw new TerraformError(
        "Failed to delete resource",
        error.message
      );
    }
  },

  async import(id: string): Promise<{ input: ResourceInput, state: ResourceState }> {
    try {
      const response = await apiClient.get(` + "`/resources/${id}`" + `);
      const state = response.data;
      
      // Extract input from the state
      const input: ResourceInput = {
        name: state.name,
        description: state.description,
        tags: state.tags
      };
      
      return { input, state };
    } catch (error) {
      throw new TerraformError(
        "Failed to import resource",
        error.message
      );
    }
  }
};

export default resource;
` + "```" + `

This example demonstrates how to implement a custom resource that interacts with an external API. It includes input validation, error handling, and the full lifecycle of a resource.

#### Logging

The Custom Provider offers logging capabilities to help debug your Node.js scripts. You can define the ` + "`log_file`" + ` attribute to specify a file where logs will be written. If not set, the provider will not log to a file.

The log file will contain the stdout and stderr output of the Node.js process.

#### Module Support

The Custom Provider supports both CommonJS and ES module formats:
- **CommonJS**: Use ` + "`module.exports`" + ` to expose your resource implementation.
` + "```" + `javascript
module.exports = { /* resource implementation */ };
` + "```" + `

- **ES Modules**: Use ` + "`export default`" + ` to expose your resource implementation.
` + "```" + `javascript
export default { /* resource implementation */ };
` + "```" + `
Choose the format that best fits your project structure and preferences.

The file type is determined by the file extension:
- ` + "`.js`" + `, ` + "`.cjs`" + `: CommonJS
- ` + "`.mjs`" + `: ES Module

#### Error Handling

Any error that is thrown will be shown in the Terraform output. It is recommended although to use the ` + "`TerraformError`" + ` class to throw errors. This class allows you to set a summary and detail message for the error that will be shown in the Terraform output.

#### Environment Variables

You can set environment variables for your Node.js script using the ` + "`environment_variables`" + ` attribute. This allows you to pass sensitive information, such as API tokens or database credentials, without hardcoding them in your script.

Apart from the set environment variables, the Node.js process will always inherit the environment variables from the Terraform process.

#### CWD

The Node.js script will always be executed in the directory in which the script is located. This means that relative paths in your script will be resolved relative to the script's location.

#### Timeouts

Two timeout settings help you control execution time limits:
- **` + "`initialize_timeout`" + `**: (Optional) The maximum time allowed for resource initialization. Defaults to 5 seconds. Format follows Go's duration syntax (e.g., "5s", "1m", "500ms").
- **` + "`operation_timeout`" + `**: (Optional) The maximum time allowed for each resource operation (create, read, update, delete). Defaults to 30 seconds. Format follows Go's duration syntax.

`,
			},
			"nodejs_data_source": schema.SetNestedBlock{
				NestedObject:        nodejsScriptBlock,
				MarkdownDescription: "This defines a Node.js data source. This is a read-only resource that fetches information from an external system using a Node.js script. It is implemented the same way as the resource, but only implements the `read` operation.",
			},
		},
		MarkdownDescription: `The Custom Terraform Provider allows you to extend Terraform by writing your own resources and data sources using simple scripting languages. This provider serves as a bridge between Terraform and your custom scripts, enabling you to implement custom infrastructure logic without needing to develop a full Terraform provider from scratch.

### Overview

This provider empowers you to:
- Create custom Terraform resources using scripting languages
- Define your own implementation logic for resource operations (create, read, update, delete)
- Build data sources that fetch information from external systems
- Integrate with systems that don't have dedicated Terraform providers

### Supported Languages

Currently, the Custom Provider supports:
- **[Node.js](https://nodejs.org/)**: Write your resources and data sources using JavaScript

Support for additional scripting languages may be added in the future. Contributions are welcome via pull requests on GitHub!

### Key Features
- **Simplicity**: Focus on your infrastructure logic without dealing with the complexity of developing a full provider
- **Flexibility**: Implement custom resources and data sources for any system with API access
- **Configurability**: Set timeouts, custom executable paths, and other options for each resource
- **Logging**: Optional logging capabilities to help troubleshoot your scripts`,
	}
}

func (p *CustomProvider) Configure(ctx context.Context, request provider.ConfigureRequest, response *provider.ConfigureResponse) {
	var data CustomProviderModel

	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)

	if response.Diagnostics.HasError() {
		return
	}

	resourceConnectors := map[string]connector.ResourceConnector{}
	dataSourceConnectors := map[string]connector.DataSourceConnector{}

	for _, nodejsResource := range data.NodejsResources {
		resourceConnector, err := nodejs.NewNodejsResourceConnector(
			nodejsResource.getNodejsExecutable(),
			nodejsResource.Args,
			nodejsResource.Path,
			nodejsResource.getInitializeTimeout(),
			nodejsResource.getOperationTimeout(),
			nodejsResource.getLogFile(),
			nodejsResource.getEnvironmentVariables(),
			nodejsResource.getInputType())
		if err != nil {
			response.Diagnostics.AddError(err.Summary, err.Detail)
			return
		}

		name := resourceConnector.Name()
		if resourceConnectors[name] != nil {
			response.Diagnostics.AddError("Duplicate resource connector", "Resource connector with name "+name+" already exists")
			return
		}

		resourceConnectors[name] = resourceConnector
	}

	for _, nodejsDataSource := range data.NodejsDataSources {
		dataSourceConnector, err := nodejs.NewNodejsDataSourceConnector(
			nodejsDataSource.getNodejsExecutable(),
			nodejsDataSource.Args,
			nodejsDataSource.Path,
			nodejsDataSource.getInitializeTimeout(),
			nodejsDataSource.getOperationTimeout(),
			nodejsDataSource.getLogFile(),
			nodejsDataSource.getEnvironmentVariables(),
			nodejsDataSource.getInputType())
		if err != nil {
			response.Diagnostics.AddError(err.Summary, err.Detail)
			return
		}

		name := dataSourceConnector.Name()
		if dataSourceConnectors[name] != nil {
			response.Diagnostics.AddError("Duplicate data source connector", "Data source connector with name "+name+" already exists")
			return
		}

		dataSourceConnectors[name] = dataSourceConnector
	}

	response.ResourceData = &resourceConnectors
	response.DataSourceData = &dataSourceConnectors
}

func (p *CustomProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewCustomResource,
	}
}

func (p *CustomProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewCustomDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &CustomProvider{
			version: version,
		}
	}
}
