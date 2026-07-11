# The bare form resolves the project like create: the provider default,
# else the API key's binding.
terraform import kernel_extension.example <extension-id>

# The project-qualified form imports an extension from a specific project.
terraform import kernel_extension.example <project-id>/<extension-id>
