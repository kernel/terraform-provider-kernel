# The bare form resolves the project like create: the provider default,
# else the API key's binding.
terraform import kernel_browser_pool.example <browser-pool-id>

# The project-qualified form imports a pool from a specific project.
terraform import kernel_browser_pool.example <project-id>/<browser-pool-id>
