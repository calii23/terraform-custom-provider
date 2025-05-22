package customvalidator

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework-validators/helpers/validatordiag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"time"
)

var _ validator.String = validDuration{}

type validDuration struct {
}

func (v validDuration) Description(context.Context) string {
	return "value must be a valid duration string"
}

func (v validDuration) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v validDuration) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	value := request.ConfigValue.ValueString()
	if _, err := time.ParseDuration(value); err != nil {
		response.Diagnostics.Append(validatordiag.InvalidAttributeValueDiagnostic(
			request.Path,
			v.Description(ctx),
			value,
		))
	}
}

func ValidDuration() validDuration {
	return validDuration{}
}
