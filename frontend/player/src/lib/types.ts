// Mirrors the JSON shapes served by the Go API (internal/auth, player, item,
// reward, ranking). See docs/api-spec.md.

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

export interface Player {
  id: number;
  user_id: number;
  player_name: string;
  level: number;
  experience: number;
  coins: number;
  created_at: string;
  updated_at: string;
}

/** The slice of a player that item use and reward claims change. */
export interface Progress {
  level: number;
  experience: number;
  coins: number;
}

export type ItemType = "coin" | "experience";

export interface InventoryItem {
  item_id: number;
  name: string;
  description: string;
  item_type: ItemType;
  effect_value: number;
  quantity: number;
}

export interface UseItemResult {
  transaction_id: number;
  item_id: number;
  quantity: number;
  replayed: boolean;
  player: Progress;
}

export interface RewardStatus {
  reward_date: string;
  claimed: boolean;
  coins: number;
  experience: number;
}

export interface ClaimResult {
  claim_id: number;
  reward_date: string;
  coins: number;
  experience: number;
  player: Progress;
}

export interface RankingEntry {
  rank: number;
  player_id: number;
  player_name: string;
  level: number;
  experience: number;
}

export interface RankingPage {
  rankings: RankingEntry[];
  limit: number;
  offset: number;
  total: number;
}

export interface MyRank extends RankingEntry {
  total: number;
}
