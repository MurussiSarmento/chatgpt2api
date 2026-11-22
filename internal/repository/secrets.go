package repository

import (
	"context"

	"github.com/basketikun/chatgpt2api/internal/crypto"
)

// enc/dec wrap the crypto package and fail safe: on a crypto error they return
// the original value rather than panicking. Init is required at startup, so an
// error here only happens for genuinely corrupt data or a wrong key, in which
// case the (still-encrypted) value surfaces downstream as an auth failure.
func enc(value string) string {
	ciphertext, err := crypto.Encrypt(value)
	if err != nil {
		return value
	}
	return ciphertext
}

func dec(value string) string {
	plaintext, err := crypto.Decrypt(value)
	if err != nil {
		return value
	}
	return plaintext
}

// encryptAccount encrypts the sensitive columns of an account in place before a
// struct Save. Empty values are left empty so predicates like `access_token <> ''`
// keep working.
func encryptAccount(item *Account) {
	item.AccessToken = enc(item.AccessToken)
	item.RefreshToken = enc(item.RefreshToken)
	item.IDToken = enc(item.IDToken)
	item.Proxy = enc(item.Proxy)
}

// decryptAccount reverses encryptAccount after a DB read.
func decryptAccount(item *Account) {
	item.AccessToken = dec(item.AccessToken)
	item.RefreshToken = dec(item.RefreshToken)
	item.IDToken = dec(item.IDToken)
	item.Proxy = dec(item.Proxy)
}

func decryptAccounts(items []Account) {
	for index := range items {
		decryptAccount(&items[index])
	}
}

// encryptAccountValues encrypts the sensitive keys present in a map update.
func encryptAccountValues(values map[string]any) {
	for _, key := range []string{"access_token", "refresh_token", "id_token", "proxy"} {
		if raw, ok := values[key]; ok {
			if text, ok := raw.(string); ok {
				values[key] = enc(text)
			}
		}
	}
}

// EncryptPlaintextSecrets is an idempotent startup backfill that encrypts any
// legacy plaintext secret still stored in the clear (accounts, api_keys, cpas,
// subs). It reads rows directly (bypassing the decrypt-on-read helpers) and
// skips values already carrying the ciphertext marker, so it is a no-op on a
// fresh or already-migrated database.
func (r *Repository) EncryptPlaintextSecrets(ctx context.Context) error {
	var accounts []Account
	if err := r.db.WithContext(ctx).Find(&accounts).Error; err != nil {
		return err
	}
	for _, item := range accounts {
		updates := map[string]any{}
		for field, value := range map[string]string{"access_token": item.AccessToken, "refresh_token": item.RefreshToken, "id_token": item.IDToken, "proxy": item.Proxy} {
			if value != "" && !crypto.IsEncrypted(value) {
				updates[field] = enc(value)
			}
		}
		if len(updates) > 0 {
			r.db.WithContext(ctx).Model(&Account{}).Where("id = ?", item.ID).Updates(updates)
		}
	}

	var keys []APIKey
	if err := r.db.WithContext(ctx).Find(&keys).Error; err != nil {
		return err
	}
	for _, item := range keys {
		updates := map[string]any{}
		if !crypto.IsEncrypted(item.Key) {
			updates["key_hash"] = crypto.Hash(item.Key)
			updates["key"] = enc(item.Key)
		} else if item.KeyHash == "" {
			if plain, err := crypto.Decrypt(item.Key); err == nil {
				updates["key_hash"] = crypto.Hash(plain)
			}
		}
		if len(updates) > 0 {
			r.db.WithContext(ctx).Model(&APIKey{}).Where("id = ?", item.ID).Updates(updates)
		}
	}

	var cpas []CPA
	if err := r.db.WithContext(ctx).Find(&cpas).Error; err != nil {
		return err
	}
	for _, item := range cpas {
		if item.SecretKey != "" && !crypto.IsEncrypted(item.SecretKey) {
			r.db.WithContext(ctx).Model(&CPA{}).Where("id = ?", item.ID).Update("secret_key", enc(item.SecretKey))
		}
	}

	var subs []Sub
	if err := r.db.WithContext(ctx).Find(&subs).Error; err != nil {
		return err
	}
	for _, item := range subs {
		updates := map[string]any{}
		if item.Password != "" && !crypto.IsEncrypted(item.Password) {
			updates["password"] = enc(item.Password)
		}
		if item.APIKey != "" && !crypto.IsEncrypted(item.APIKey) {
			updates["api_key"] = enc(item.APIKey)
		}
		if len(updates) > 0 {
			r.db.WithContext(ctx).Model(&Sub{}).Where("id = ?", item.ID).Updates(updates)
		}
	}
	return nil
}
