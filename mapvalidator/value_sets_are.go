package mapvalidator

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// ValueSetsAre returns an validator which ensures that any configured
// Set values passes each Set validator.
//
// Null (unconfigured) and unknown (known after apply) values are skipped.
func ValueSetsAre(elementValidators ...validator.Set) validator.Map {
	return valueSetsAreValidator{
		elementValidators: elementValidators,
	}
}

var _ validator.Map = valueSetsAreValidator{}

// valueSetsAreValidator validates that each set member validates against each of the value validators.
type valueSetsAreValidator struct {
	elementValidators []validator.Set
}

// Description describes the validation in plain text formatting.
func (v valueSetsAreValidator) Description(ctx context.Context) string {
	var descriptions []string

	for _, elementValidator := range v.elementValidators {
		descriptions = append(descriptions, elementValidator.Description(ctx))
	}

	return fmt.Sprintf("element value must satisfy all validations: %s", strings.Join(descriptions, " + "))
}

// MarkdownDescription describes the validation in Markdown formatting.
func (v valueSetsAreValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateSet performs the validation.
func (v valueSetsAreValidator) ValidateMap(ctx context.Context, req validator.MapRequest, resp *validator.MapResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	_, ok := req.ConfigValue.ElementType(ctx).(basetypes.SetTypable)

	if !ok {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Validator for Element Type",
			"While performing schema-based validation, an unexpected error occurred. "+
				"The attribute declares a Set values validator, however its values do not implement types.SetType or the types.SetTypable interface for custom Set types. "+
				"Use the appropriate values validator that matches the element type. "+
				"This is always an issue with the provider and should be reported to the provider developers.\n\n"+
				fmt.Sprintf("Path: %s\n", req.Path.String())+
				fmt.Sprintf("Element Type: %T\n", req.ConfigValue.ElementType(ctx)),
		)

		return
	}

	for key, element := range req.ConfigValue.Elements() {
		elementPath := req.Path.AtMapKey(key)

		elementValuable, ok := element.(basetypes.SetValuable)

		// The check above should have prevented this, but raise an error
		// instead of a type assertion panic or skipping the element. Any issue
		// here likely indicates something wrong in the framework itself.
		if !ok {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid Validator for Element Value",
				"While performing schema-based validation, an unexpected error occurred. "+
					"The attribute declares a Set values validator, however its values do not implement types.SetType or the types.SetTypable interface for custom Set types. "+
					"This is likely an issue with terraform-plugin-framework and should be reported to the provider developers.\n\n"+
					fmt.Sprintf("Path: %s\n", req.Path.String())+
					fmt.Sprintf("Element Type: %T\n", req.ConfigValue.ElementType(ctx))+
					fmt.Sprintf("Element Value Type: %T\n", element),
			)

			return
		}

		elementValue, diags := elementValuable.ToSetValue(ctx)

		resp.Diagnostics.Append(diags...)

		// Only return early if the new diagnostics indicate an issue since
		// it likely will be the same for all elements.
		if diags.HasError() {
			return
		}

		elementReq := validator.SetRequest{
			Path:           elementPath,
			PathExpression: elementPath.Expression(),
			ConfigValue:    elementValue,
			Config:         req.Config,
		}

		for _, elementValidator := range v.elementValidators {
			elementResp := &validator.SetResponse{}

			elementValidator.ValidateSet(ctx, elementReq, elementResp)

			resp.Diagnostics.Append(elementResp.Diagnostics...)
		}
	}
}
