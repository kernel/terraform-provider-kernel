# Terraform Framework Code Generation Decision

## Decision

Keep Terraform schemas and models handwritten for v1. Do not adopt
`tfplugingen-framework` or add a code-generation drift check.

HashiCorp currently labels provider code generation a tech preview. Version
v0.4.1 was the latest release when evaluated on 2026-07-10.

References:

- [Framework code generator](https://developer.hashicorp.com/terraform/plugin/code-generation/framework-generator)
- [Provider code specification](https://developer.hashicorp.com/terraform/plugin/code-generation/specification)

## Prototype Result

A disposable prototype generated the existing `kernel_extension` data-source
schema and Terraform model from a handwritten Provider Code Specification.

The generator correctly reproduced:

- all six attributes;
- required, optional, and computed flags;
- Markdown descriptions;
- the `project_id` length validator;
- package name `extension`;
- a Terraform model with matching `tfsdk` tags.

The generated shape still failed the canary's complexity test:

- 70 lines of checked-in specification;
- 64 lines of generated Go;
- 17 lines for the pinned regeneration command;
- 151 total lines to replace 57 lines of direct schema and model code.

The generated model also uses `Id` and `ProjectId` rather than the existing
idiomatic `ID` and `ProjectID`, creating mechanical churn across the extension
package. Generated attributes set both plain and Markdown descriptions, while
the provider currently needs only Markdown descriptions.

The output is valid, but it does not delete complexity. It adds a second source
representation, generated output, tool boot/download cost, a future CI drift
check, and naming churn for one small flat schema.

## Ownership After Rejection

The existing package boundaries remain unchanged:

- `internal/datasources/extension` owns its direct schema, model, lookup, and
  tests;
- `internal/provider` owns registration;
- `internal/kernelclient` owns durable SDK calls only;
- Terraform null, unknown, sensitive, lookup, import, and lifecycle semantics
  remain handwritten;
- OpenAPI, Stainless, and SDK types do not generate Terraform behavior.

No generator binary, Provider Code Specification, generated Go, wrapper, tool
module, or CI step is added to the repository.

## Reconsideration Criteria

Re-evaluate code generation only if the provider later has enough repeated
nested schema shape that a prototype proves all of these:

- generated plus specification code is materially smaller or easier to review
  than direct Go;
- generated names preserve idiomatic Go initialisms without wrapper types;
- the tool is stable enough for routine upgrades;
- output is deterministic on a clean checkout;
- ordinary unit tests remain independent of the generator and network;
- any drift check is cached, credential-free, Docker-free, and measurably fast;
- generated types stay inside their resource or data-source package;
- CRUD, import, project scope, sensitive state, retries, clear semantics, and
  runtime exclusions remain handwritten.

Until those conditions are demonstrated, issue #25 is rejected for v1 rather
than left as hidden follow-up work.

This measured decision supersedes the v0-era `v1/future` classification in
`docs/concerns.md`. Future reconsideration remains possible only under the
criteria above; code generation is not part of the v1 work list.
