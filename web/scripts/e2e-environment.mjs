export function playwrightEnvironment(source) {
  const environment = { ...source }
  delete environment.NO_COLOR
  return environment
}
