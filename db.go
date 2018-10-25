package breez

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"

	bolt "go.etcd.io/bbolt"
)

const (
	incmoingPayReqBucket   = "paymentRequests"
	addressesBucket        = "swap_addresses"
	paymentsBucket         = "payments"
	paymentsSyncInfoBucket = "paymentsSyncInfo"
	accountBucket          = "account"
)

var db *bolt.DB

func openDB(dbPath string) error {
	var err error
	db, err = bolt.Open(dbPath, 0600, nil)
	if err != nil {
		log.Criticalf("Failed to open database %v", err)
		return err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		var err error
		_, err = tx.CreateBucketIfNotExists([]byte(incmoingPayReqBucket))
		if err != nil {
			return err
		}
		paymenetBucket, err := tx.CreateBucketIfNotExists([]byte(paymentsBucket))
		if err != nil {
			return err
		}
		_, err = paymenetBucket.CreateBucketIfNotExists([]byte(paymentsSyncInfoBucket))
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists([]byte(accountBucket))
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists([]byte(addressesBucket))
		return err
	})
	if err != nil {
		log.Criticalf("Failed to create buckets: %v", err)
		return err
	}
	return nil
}

func closeDB() error {
	return db.Close()
}

func deleteDB() error {
	return os.Remove(db.Path())
}

func addAccountPayment(accPayment []byte, receivedIndex uint64, sentTime uint64) error {

	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(paymentsBucket))
		id, err := b.NextSequence()
		if err != nil {
			return err
		}

		//write the payment value with the next sequence as key
		if err := b.Put(itob(id), accPayment); err != nil {
			return err
		}
		syncInfoBucket := b.Bucket([]byte(paymentsSyncInfoBucket))

		//if we have a newer item, update the last payment timestamp
		lastPaymentTime := uint64(0)
		if lastPaymentTimeBuf := syncInfoBucket.Get([]byte("lastSentPaymentTime")); lastPaymentTimeBuf != nil {
			lastPaymentTime = btoi(lastPaymentTimeBuf)
		}
		if lastPaymentTime < sentTime {
			if err := syncInfoBucket.Put([]byte("lastSentPaymentTime"), itob(sentTime)); err != nil {
				return err
			}
		}

		lastInvoiceSettledIndex := uint64(0)
		if lastInvoiceSettledIndexBuf := syncInfoBucket.Get([]byte("lastSettledIndex")); lastInvoiceSettledIndexBuf != nil {
			lastInvoiceSettledIndex = btoi(lastInvoiceSettledIndexBuf)
		}
		if lastInvoiceSettledIndex < receivedIndex {
			if err := syncInfoBucket.Put([]byte("lastSettledIndex"), itob(receivedIndex)); err != nil {
				return err
			}
		}
		return nil
	})
}

func fetchAllAccountPayments() ([][]byte, error) {
	var payments [][]byte
	err := db.View(func(tx *bolt.Tx) error {
		// Assume bucket exists and has keys
		b := tx.Bucket([]byte(paymentsBucket))

		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			if v == nil {
				//nested bucket
				continue
			}
			payments = append(payments, v)
		}

		return nil
	})
	return payments, err
}

func fetchPaymentsSyncInfo() (lastTime int64, lastSetteledIndex uint64) {
	lastPaymentTime := int64(0)
	lastInvoiceSettledIndex := uint64(0)
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(paymentsBucket))
		syncInfoBucket := b.Bucket([]byte(paymentsSyncInfoBucket))
		lastPaymentTimeBuf := syncInfoBucket.Get([]byte("lastSentPaymentTime"))
		if lastPaymentTimeBuf != nil {
			lastPaymentTime = int64(btoi(lastPaymentTimeBuf))
		}
		lastInvoiceSettledIndexBuf := syncInfoBucket.Get([]byte("lastSettledIndex"))
		if lastInvoiceSettledIndexBuf != nil {
			lastInvoiceSettledIndex = btoi(lastInvoiceSettledIndexBuf)
		}
		return nil
	})
	return lastPaymentTime, lastInvoiceSettledIndex
}

func saveAccount(account []byte) error {
	return saveItem([]byte(accountBucket), []byte("account"), account)
}

func fetchAccount() ([]byte, error) {
	return fetchItem([]byte(accountBucket), []byte("account"))
}

func savePaymentRequest(payReqHash string, payReq []byte) error {
	return saveItem([]byte(incmoingPayReqBucket), []byte(payReqHash), payReq)
}

func fetchPaymentRequest(payReqHash string) ([]byte, error) {
	return fetchItem([]byte(incmoingPayReqBucket), []byte(payReqHash))
}

/**
Swap addresses
**/
func fetchAllSwapAddresses() ([]*swapAddressInfo, error) {
	return fetchSwapAddresses(func(addr *swapAddressInfo) bool {
		return true
	})
}

func fetchSwapAddresses(filterFunc func(addr *swapAddressInfo) bool) ([]*swapAddressInfo, error) {
	var addresses []*swapAddressInfo
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(addressesBucket))
		b.ForEach(func(k, v []byte) error {
			var address swapAddressInfo
			err := json.Unmarshal(v, &address)
			if err != nil {
				return err
			}
			if filterFunc(&address) {
				addresses = append(addresses, &address)
			}
			return nil
		})
		return nil
	})
	return addresses, err
}

func saveSwapAddressInfo(address *swapAddressInfo) error {
	bytes, err := serializeSwapAddressInfo(address)
	if err != nil {
		return err
	}
	return saveItem([]byte(addressesBucket), []byte(address.Address), bytes)
}

func removeSwapAddress(address string) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(addressesBucket))
		return b.Delete([]byte(address))
	})
}

func removeSwapAddressByPaymentHash(pHash []byte) (bool, error) {
	var found bool
	err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(addressesBucket))
		return b.ForEach(func(k, v []byte) error {
			address, err := deserializeSwapAddressInfo(v)
			if err != nil {
				return err
			}
			if bytes.Equal(address.PaymentHash, pHash) {
				found = true
				return b.Delete(k)
			}
			return nil
		})
	})
	return found, err
}

func updateSwapAddressInfo(address string, updateFund func(*swapAddressInfo)) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(addressesBucket))
		addressInfoBytes := b.Get([]byte(address))
		var decodedAddressInfo swapAddressInfo
		err := json.Unmarshal(addressInfoBytes, &decodedAddressInfo)
		if err != nil {
			return err
		}
		updateFund(&decodedAddressInfo)
		addressInfoBytes, err = json.Marshal(decodedAddressInfo)
		if err != nil {
			return err
		}

		return b.Put([]byte(address), addressInfoBytes)
	})
}

func saveItem(bucket []byte, key []byte, value []byte) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		return b.Put(key, value)
	})
}

func fetchItem(bucket []byte, key []byte) ([]byte, error) {
	var value []byte
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		value = b.Get(key)
		return nil
	})
	return value, err
}

func itob(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func btoi(bytes []byte) uint64 {
	return binary.BigEndian.Uint64(bytes)
}