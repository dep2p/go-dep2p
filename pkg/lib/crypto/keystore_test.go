package crypto

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMemKeystore(t *testing.T) {
	ks := NewMemKeystore()

	priv, _, _ := GenerateKeyPair(KeyTypeEd25519)

	t.Run("Has_NotExists", func(t *testing.T) {
		has, err := ks.Has("test")
		if err != nil {
			t.Fatalf("Has() error = %v", err)
		}
		if has {
			t.Error("Has() = true, want false")
		}
	})

	t.Run("Put", func(t *testing.T) {
		err := ks.Put("test", priv)
		if err != nil {
			t.Fatalf("Put() error = %v", err)
		}
	})

	t.Run("Has_Exists", func(t *testing.T) {
		has, _ := ks.Has("test")
		if !has {
			t.Error("Has() = false, want true")
		}
	})

	t.Run("Put_Duplicate", func(t *testing.T) {
		err := ks.Put("test", priv)
		if err != ErrKeyExists {
			t.Errorf("Put(duplicate) error = %v, want ErrKeyExists", err)
		}
	})

	t.Run("Get", func(t *testing.T) {
		got, err := ks.Get("test")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if !KeyEqual(priv, got) {
			t.Error("Get() returned different key")
		}
	})

	t.Run("Get_NotExists", func(t *testing.T) {
		_, err := ks.Get("notexists")
		if err != ErrKeyNotFound {
			t.Errorf("Get(notexists) error = %v, want ErrKeyNotFound", err)
		}
	})

	t.Run("List", func(t *testing.T) {
		ids, err := ks.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(ids) != 1 || ids[0] != "test" {
			t.Errorf("List() = %v, want [test]", ids)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := ks.Delete("test")
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		has, _ := ks.Has("test")
		if has {
			t.Error("Has() = true after Delete()")
		}
	})

	t.Run("Delete_NotExists", func(t *testing.T) {
		err := ks.Delete("notexists")
		if err != ErrKeyNotFound {
			t.Errorf("Delete(notexists) error = %v, want ErrKeyNotFound", err)
		}
	})
}

func TestFSKeystore_NoEncryption(t *testing.T) {
	dir := t.TempDir()

	ks, err := NewFSKeystore(dir, nil)
	if err != nil {
		t.Fatalf("NewFSKeystore() error = %v", err)
	}

	priv, _, _ := GenerateKeyPair(KeyTypeEd25519)

	t.Run("Put_Get", func(t *testing.T) {
		err := ks.Put("mykey", priv)
		if err != nil {
			t.Fatalf("Put() error = %v", err)
		}

		got, err := ks.Get("mykey")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		if !KeyEqual(priv, got) {
			t.Error("Get() returned different key")
		}
	})

	t.Run("FileExists", func(t *testing.T) {
		path := filepath.Join(dir, "mykey.key")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Error("Key file not created")
		}
	})

	t.Run("List", func(t *testing.T) {
		ids, err := ks.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(ids) != 1 || ids[0] != "mykey" {
			t.Errorf("List() = %v, want [mykey]", ids)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := ks.Delete("mykey")
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		path := filepath.Join(dir, "mykey.key")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Error("Key file not deleted")
		}
	})
}

func TestFSKeystore_WithEncryption(t *testing.T) {
	dir := t.TempDir()
	password := []byte("test-password-123")

	ks, err := NewFSKeystore(dir, password)
	if err != nil {
		t.Fatalf("NewFSKeystore() error = %v", err)
	}

	priv, _, _ := GenerateKeyPair(KeyTypeEd25519)

	t.Run("Put_Get", func(t *testing.T) {
		err := ks.Put("encrypted", priv)
		if err != nil {
			t.Fatalf("Put() error = %v", err)
		}

		got, err := ks.Get("encrypted")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		if !KeyEqual(priv, got) {
			t.Error("Get() returned different key")
		}
	})

	t.Run("WrongPassword", func(t *testing.T) {
		wrongKs, _ := NewFSKeystore(dir, []byte("wrong-password"))
		_, err := wrongKs.Get("encrypted")
		if err == nil {
			t.Error("Get() with wrong password should return error")
		}
	})

	t.Run("NoPassword", func(t *testing.T) {
		noPassKs, _ := NewFSKeystore(dir, nil)
		_, err := noPassKs.Get("encrypted")
		if err == nil {
			t.Error("Get() without password should return error")
		}
	})
}

func TestFSKeystore_DifferentKeyTypes(t *testing.T) {
	dir := t.TempDir()
	ks, _ := NewFSKeystore(dir, nil)

	keyTypes := []KeyType{KeyTypeEd25519, KeyTypeSecp256k1, KeyTypeECDSA, KeyTypeRSA}

	for _, kt := range keyTypes {
		t.Run(kt.String(), func(t *testing.T) {
			priv, _, err := GenerateKeyPair(kt)
			if err != nil {
				t.Fatalf("GenerateKeyPair() error = %v", err)
			}

			id := "key-" + kt.String()
			err = ks.Put(id, priv)
			if err != nil {
				t.Fatalf("Put() error = %v", err)
			}

			got, err := ks.Get(id)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}

			if !KeyEqual(priv, got) {
				t.Error("Get() returned different key")
			}

			if got.Type() != kt {
				t.Errorf("Get().Type() = %v, want %v", got.Type(), kt)
			}
		})
	}
}

func TestEncryptDecrypt(t *testing.T) {
	password := []byte("test-password")
	plaintext := []byte("secret data to encrypt")

	encrypted, err := encryptData(plaintext, password)
	if err != nil {
		t.Fatalf("encryptData() error = %v", err)
	}

	if string(encrypted) == string(plaintext) {
		t.Error("encryptData() returned plaintext")
	}

	decrypted, err := decryptData(encrypted, password)
	if err != nil {
		t.Fatalf("decryptData() error = %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("decryptData() = %q, want %q", decrypted, plaintext)
	}
}

func TestDecrypt_WrongPassword(t *testing.T) {
	password := []byte("correct-password")
	plaintext := []byte("secret data")

	encrypted, _ := encryptData(plaintext, password)

	_, err := decryptData(encrypted, []byte("wrong-password"))
	if err == nil {
		t.Error("decryptData() with wrong password should return error")
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	_, err := decryptData([]byte{1, 2, 3}, []byte("password"))
	if err == nil {
		t.Error("decryptData(short) should return error")
	}
}

func TestSecureZero(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	SecureZero(data)

	for i, b := range data {
		if b != 0 {
			t.Errorf("SecureZero() data[%d] = %d, want 0", i, b)
		}
	}
}

func TestDeriveKey(t *testing.T) {
	password := []byte("test-password")
	salt := []byte("random-salt-1234")

	key1 := DeriveKey(password, salt)
	key2 := DeriveKey(password, salt)

	if len(key1) != 32 {
		t.Errorf("DeriveKey() len = %d, want 32", len(key1))
	}

	// 相同输入应产生相同密钥
	if string(key1) != string(key2) {
		t.Error("DeriveKey() not deterministic")
	}

	// 不同盐应产生不同密钥
	key3 := DeriveKey(password, []byte("different-salt!!"))
	if string(key1) == string(key3) {
		t.Error("DeriveKey() with different salt produced same key")
	}
}
