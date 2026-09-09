export const minimumAdminPasswordCharacters = 15;

export function hasAdminLoginInput(username: string, password: string): boolean {
  return username.trim().length > 0 && password.length > 0;
}

export function hasMinimumAdminPasswordLength(password: string): boolean {
  return Array.from(password).length >= minimumAdminPasswordCharacters;
}
