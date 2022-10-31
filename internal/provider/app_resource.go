package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/floydspace/terraform-provider-fly/graphql"
	"github.com/floydspace/terraform-provider-fly/internal/utils"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

var _ resource.ResourceWithConfigure = &flyAppResource{}
var _ resource.ResourceWithImportState = &flyAppResource{}

type flyAppResourceData struct {
	Id              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Org             types.String `tfsdk:"org"`
	OrgId           types.String `tfsdk:"orgid"`
	AppUrl          types.String `tfsdk:"appurl"`
	Hostname        types.String `tfsdk:"hostname"`
	SharedIpAddress types.String `tfsdk:"sharedipaddress"`
	//Secrets types.Map    `tfsdk:"secrets"`
}

func appDataFromGraphql(f graphql.AppFragment) flyAppResourceData {
	return flyAppResourceData{
		Name:   types.String{Value: f.Name},
		Org:    types.String{Value: f.Organization.Slug},
		OrgId:  types.String{Value: f.Organization.Id},
		AppUrl: types.String{Value: f.AppUrl},
		Id:     types.String{Value: f.Id},
	}
}

func (r flyAppResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "fly_app"
}

func (r flyAppResource) GetSchema(context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Fly app resource",

		Attributes: map[string]tfsdk.Attribute{
			"name": {
				MarkdownDescription: "Name of application",
				Required:            true,
				Type:                types.StringType,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					resource.RequiresReplace(),
				},
			},
			"org": {
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "Optional org slug to operate upon",
				Type:                types.StringType,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					resource.RequiresReplace(),
				},
			},
			"orgid": {
				Computed:            true,
				MarkdownDescription: "readonly orgid",
				Type:                types.StringType,
			},
			"id": {
				Computed:            true,
				MarkdownDescription: "readonly app id",
				Type:                types.StringType,
			},
			"appurl": {
				Computed:            true,
				MarkdownDescription: "readonly appUrl",
				Type:                types.StringType,
			},
			"hostname": {
				Computed:            true,
				MarkdownDescription: "readonly hostname",
				Type:                types.StringType,
			},
			"sharedipaddress": {
				Computed:            true,
				MarkdownDescription: "readonly sharedIpAddress",
				Type:                types.StringType,
			},
			//"secrets": {
			//	Sensitive:           true,
			//	Optional:            true,
			//	MarkdownDescription: "App secrets",
			//	Type:                types.MapType{ElemType: types.StringType},
			//},
		},
	}, nil
}

func newAppResource() resource.Resource {
	return &flyAppResource{}
}

type flyAppResource struct {
	flyResource
}

func (r flyAppResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data flyAppResourceData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if data.Org.Unknown {
		defaultOrg, err := utils.GetDefaultOrg(r.gqlClient)
		if err != nil {
			resp.Diagnostics.AddError("Could not detect default organization", err.Error())
			return
		}
		data.OrgId.Value = defaultOrg.Id
		data.Org.Value = defaultOrg.Name
	} else {
		org, err := graphql.Organization(context.Background(), r.gqlClient, data.Org.Value)
		if err != nil {
			resp.Diagnostics.AddError("Could not resolve organization", err.Error())
			return
		}
		data.OrgId.Value = org.Organization.Id
	}
	mresp, err := graphql.CreateAppMutation(context.Background(), r.gqlClient, data.Name.Value, data.OrgId.Value)
	if err != nil {
		resp.Diagnostics.AddError("Create app failed", err.Error())
		return
	}

	data = appDataFromGraphql(mresp.CreateApp.App)

	//if len(data.Secrets.Elems) > 0 {
	//	var rawSecrets map[string]string
	//	data.Secrets.ElementsAs(context.Background(), &rawSecrets, false)
	//	var secrets []graphql.SecretInput
	//	for k, v := range rawSecrets {
	//		secrets = append(secrets, graphql.SecretInput{
	//			Key:   k,
	//			Value: v,
	//		})
	//	}
	//	_, err := graphql.SetSecrets(context.Background(), *r.gqlClient, graphql.SetSecretsInput{
	//		AppId:      data.Id.Value,
	//		Secrets:    secrets,
	//		ReplaceAll: true,
	//	})
	//	if err != nil {
	//		resp.Diagnostics.AddError("Could not set rawSecrets", err.Error())
	//		return
	//	}
	//	data.Secrets = utils.KVToTfMap(rawSecrets, types.StringType)
	//}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r flyAppResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state flyAppResourceData

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	query, err := graphql.GetApp(context.Background(), r.gqlClient, state.Name.Value)
	var errList gqlerror.List
	if errors.As(err, &errList) {
		for _, err := range errList {
			if err.Message == "Could not resolve " {
				return
			}
			resp.Diagnostics.AddError(err.Message, err.Path.String())
		}
	} else if err != nil {
		resp.Diagnostics.AddError("Read: query failed", err.Error())
	}

	data := appDataFromGraphql(query.App)

	//if !state.Secrets.Null && !state.Secrets.Unknown {
	//	data.Secrets = state.Secrets
	//}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r flyAppResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan flyAppResourceData

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var state flyAppResourceData
	diags = resp.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	tflog.Info(ctx, fmt.Sprintf("existing: %+v, new: %+v", state, plan))

	if !plan.Org.Unknown && plan.Org.Value != state.Org.Value {
		resp.Diagnostics.AddError("Can't mutate org of existing app", "Can't switch org"+state.Org.Value+" to "+plan.Org.Value)
	}
	if !plan.Name.Null && plan.Name.Value != state.Name.Value {
		resp.Diagnostics.AddError("Can't mutate Name of existing app", "Can't switch name "+state.Name.Value+" to "+plan.Name.Value)
	}

	//if len(plan.Secrets.Elems) > 0 {
	//	var rawSecrets map[string]string
	//	plan.Secrets.ElementsAs(context.Background(), &rawSecrets, false)
	//	var secrets []graphql.SecretInput
	//	for k, v := range rawSecrets {
	//		secrets = append(secrets, graphql.SecretInput{
	//			Key:   k,
	//			Value: v,
	//		})
	//	}
	//	_, err := graphql.SetSecrets(context.Background(), r.gqlClient, graphql.SetSecretsInput{
	//		AppId:      state.Id.Value,
	//		Secrets:    secrets,
	//		ReplaceAll: true,
	//	})
	//	if err != nil {
	//		resp.Diagnostics.AddError("Could not set rawSecrets", err.Error())
	//		return
	//	}
	//	state.Secrets = utils.KVToTfMap(rawSecrets, types.StringType)
	//}

	resp.State.Set(ctx, state)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (r flyAppResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data flyAppResourceData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	_, err := graphql.DeleteAppMutation(context.Background(), r.gqlClient, data.Name.Value)
	var errList gqlerror.List
	if errors.As(err, &errList) {
		for _, err := range errList {
			resp.Diagnostics.AddError(err.Message, err.Path.String())
		}
	} else if err != nil {
		resp.Diagnostics.AddError("Delete app failed", err.Error())
	}

	resp.State.RemoveResource(ctx)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (r flyAppResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
