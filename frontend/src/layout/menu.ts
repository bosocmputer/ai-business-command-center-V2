export type NavigationItem = {
  label: string;
  icon?: string;
  to?: string;
  activePrefix?: string;
  visible?: boolean;
  disabled?: boolean;
  badge?: string | number;
  badgeSeverity?: 'danger' | 'warn' | 'info' | 'success' | 'secondary';
  items?: NavigationItem[];
};
