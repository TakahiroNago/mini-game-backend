// Mirrors the JSON shapes served by the Go API (internal/admin, internal/item).

export type Role = "player" | "admin";

export interface AuthUser {
  id: number;
  email: string;
  role: Role;
  created_at: string;
}

export interface LoginResponse {
  access_token: string;
  token_type: "Bearer";
  expires_at: string;
  user: AuthUser;
}

export type ItemType = "coin" | "experience";

export interface Item {
  id: number;
  name: string;
  description: string;
  item_type: ItemType;
  effect_value: number;
  starter_quantity: number;
  deleted_at?: string;
  created_at: string;
  updated_at: string;
}

export interface ItemInput {
  name: string;
  description: string;
  item_type: ItemType;
  effect_value: number;
  starter_quantity: number;
}

export interface PlayerSummary {
  id: number;
  player_name: string;
  level: number;
  experience: number;
  coins: number;
}

export interface UserSummary {
  id: number;
  email: string;
  role: Role;
  created_at: string;
  player: PlayerSummary | null;
}

export interface UsersPage {
  users: UserSummary[];
  limit: number;
  offset: number;
  total: number;
}

export interface InventoryItem {
  item_id: number;
  name: string;
  description: string;
  item_type: ItemType;
  effect_value: number;
  quantity: number;
}

export interface TransactionView {
  id: number;
  item_id: number;
  item_name: string;
  quantity: number;
  transaction_type: "acquire" | "use";
  idempotency_key: string | null;
  created_at: string;
}

export interface UserDetail {
  user: UserSummary;
  inventory: InventoryItem[];
  recent_transactions: TransactionView[];
  reward_claims: number;
}

export interface RankingEntry {
  rank: number;
  player_id: number;
  player_name: string;
  level: number;
  experience: number;
}

export interface Dashboard {
  users: number;
  players: number;
  items: number;
  registrations_today: number;
  reward_claims_today: number;
  item_uses_today: number;
  reward_date: string;
  top_players: RankingEntry[];
}

export interface GrantResult {
  player_id: number;
  item_id: number;
  quantity: number;
  transaction_id: number;
}

export interface AuditLog {
  id: number;
  actor_user_id: number;
  actor_email: string;
  action: string;
  target_type: string;
  target_id: number | null;
  detail: unknown;
  created_at: string;
}

export interface AuditPage {
  logs: AuditLog[];
  limit: number;
  offset: number;
  total: number;
}
