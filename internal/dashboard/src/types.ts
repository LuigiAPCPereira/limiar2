export type ChannelStats = {
  ChannelID: number;
  Username: string;
  MessageCount: number;
};

export type RawMessage = {
  ID: number;
  ChannelID: number;
  MessageID: number;
  Payload: string;
  ReceivedAt: string;
  SchemaVersion: number;
};

export type ProcessedMessage = {
  id: number;
  raw_message_id: number;
  channel_id: number;
  message_id: number;
  message_type: string;
  text_clean: string;
  text_length: number;
  media_type: string;
  photo_id: number;
  has_url: boolean;
  has_price: boolean;
  has_coupon: boolean;
  price_amount: number;
  price_currency: string;
  posted_at: string;
  processed_at: string;
  price_original: number;
  price_discount: number;
  coupon_code: string;
  payment_method: string;
  shipping: string;
  installments: string;
  shipping_free: boolean;
  installments_n: number;
  installments_value: number;
  discount_percent: number;
  merchant: string;
  url: string;
  product_name: string;
  is_duplicate: boolean;
  is_recurring: boolean;
};

export type ProcessedStats = {
  total: number;
  by_type: { message_type: string; count: number }[];
};

export type HealthStats = {
  status: string;
  db: boolean;
  raw_messages: number;
  uptime_seconds: number;
};
