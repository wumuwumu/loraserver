package api

import (
	"context"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/band"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/downlink/data/classb"
	"github.com/brocaar/loraserver/internal/gps"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	loraband "github.com/brocaar/lorawan/band"
)

type NetworkServerAPITestSuite struct {
	suite.Suite
	api ns.NetworkServerServiceServer
}

func (ts *NetworkServerAPITestSuite) SetupSuite() {
	assert := require.New(ts.T())
	conf := test.GetConfig()
	assert.NoError(storage.Setup(conf))
	test.MustResetDB(storage.DB().DB)
	ts.api = NewNetworkServerAPI()
}

func (ts *NetworkServerAPITestSuite) SetupTest() {
	test.MustFlushRedis(storage.RedisPool())
}

func (ts *NetworkServerAPITestSuite) TestMulticastGroup() {
	assert := require.New(ts.T())

	var rp storage.RoutingProfile
	var sp storage.ServiceProfile

	assert.NoError(storage.CreateRoutingProfile(storage.DB(), &rp))
	assert.NoError(storage.CreateServiceProfile(storage.DB(), &sp))

	mg := ns.MulticastGroup{
		McAddr:           []byte{1, 2, 3, 4},
		McNwkSKey:        []byte{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
		FCnt:             10,
		GroupType:        ns.MulticastGroupType_CLASS_B,
		Dr:               5,
		Frequency:        868300000,
		PingSlotPeriod:   16,
		RoutingProfileId: rp.ID[:],
		ServiceProfileId: sp.ID[:],
	}

	ts.T().Run("Create", func(t *testing.T) {
		assert := require.New(t)
		createResp, err := ts.api.CreateMulticastGroup(context.Background(), &ns.CreateMulticastGroupRequest{
			MulticastGroup: &mg,
		})
		assert.Nil(err)
		assert.Len(createResp.Id, 16)
		assert.NotEqual(uuid.Nil.Bytes(), createResp.Id)
		mg.Id = createResp.Id

		t.Run("Get", func(t *testing.T) {
			assert := require.New(t)
			getResp, err := ts.api.GetMulticastGroup(context.Background(), &ns.GetMulticastGroupRequest{
				Id: createResp.Id,
			})
			assert.Nil(err)
			assert.NotNil(getResp.MulticastGroup)
			assert.NotNil(getResp.CreatedAt)
			assert.NotNil(getResp.UpdatedAt)
			assert.Equal(&mg, getResp.MulticastGroup)
		})

		t.Run("Update", func(t *testing.T) {
			assert := require.New(t)

			mgUpdated := ns.MulticastGroup{
				Id:               createResp.Id,
				McAddr:           []byte{4, 3, 2, 1},
				McNwkSKey:        []byte{8, 7, 6, 5, 4, 3, 2, 1, 8, 7, 6, 5, 4, 3, 2, 1},
				FCnt:             20,
				GroupType:        ns.MulticastGroupType_CLASS_C,
				Dr:               3,
				Frequency:        868100000,
				PingSlotPeriod:   32,
				RoutingProfileId: rp.ID[:],
				ServiceProfileId: sp.ID[:],
			}

			_, err := ts.api.UpdateMulticastGroup(context.Background(), &ns.UpdateMulticastGroupRequest{
				MulticastGroup: &mgUpdated,
			})
			assert.Nil(err)

			getResp, err := ts.api.GetMulticastGroup(context.Background(), &ns.GetMulticastGroupRequest{
				Id: createResp.Id,
			})
			assert.Nil(err)
			assert.Equal(&mgUpdated, getResp.MulticastGroup)
		})

		t.Run("Delete", func(t *testing.T) {
			assert := require.New(t)

			_, err := ts.api.DeleteMulticastGroup(context.Background(), &ns.DeleteMulticastGroupRequest{
				Id: createResp.Id,
			})
			assert.Nil(err)

			_, err = ts.api.DeleteMulticastGroup(context.Background(), &ns.DeleteMulticastGroupRequest{
				Id: createResp.Id,
			})
			assert.NotNil(err)
			assert.Equal(codes.NotFound, grpc.Code(err))

			_, err = ts.api.GetMulticastGroup(context.Background(), &ns.GetMulticastGroupRequest{
				Id: createResp.Id,
			})
			assert.NotNil(err)
			assert.Equal(codes.NotFound, grpc.Code(err))
		})
	})
}

func (ts *NetworkServerAPITestSuite) TestMulticastQueue() {
	assert := require.New(ts.T())

	gateways := []storage.Gateway{
		{
			GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
		},
		{
			GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 2},
		},
	}
	for i := range gateways {
		assert.NoError(storage.CreateGateway(storage.DB(), &gateways[i]))
	}

	var rp storage.RoutingProfile
	var sp storage.ServiceProfile
	var dp storage.DeviceProfile

	assert.NoError(storage.CreateRoutingProfile(storage.DB(), &rp))
	assert.NoError(storage.CreateServiceProfile(storage.DB(), &sp))
	assert.NoError(storage.CreateDeviceProfile(storage.DB(), &dp))

	devices := []storage.Device{
		{
			DevEUI:           lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 1},
			RoutingProfileID: rp.ID,
			ServiceProfileID: sp.ID,
			DeviceProfileID:  dp.ID,
		},
		{
			DevEUI:           lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
			RoutingProfileID: rp.ID,
			ServiceProfileID: sp.ID,
			DeviceProfileID:  dp.ID,
		},
	}
	for i := range devices {
		assert.NoError(storage.CreateDevice(storage.DB(), &devices[i]))
		assert.NoError(storage.SaveDeviceGatewayRXInfoSet(storage.RedisPool(), storage.DeviceGatewayRXInfoSet{
			DevEUI: devices[i].DevEUI,
			DR:     3,
			Items: []storage.DeviceGatewayRXInfo{
				{
					GatewayID: gateways[i].GatewayID,
					RSSI:      50,
					LoRaSNR:   5,
				},
			},
		}))
	}

	ts.T().Run("Class-B", func(t *testing.T) {
		assert := require.New(t)

		mg := storage.MulticastGroup{
			GroupType:        storage.MulticastGroupB,
			MCAddr:           lorawan.DevAddr{1, 2, 3, 4},
			PingSlotPeriod:   32 * 128, // every 128 seconds
			ServiceProfileID: sp.ID,
			RoutingProfileID: rp.ID,
		}
		assert.NoError(storage.CreateMulticastGroup(storage.DB(), &mg))

		for _, d := range devices {
			assert.NoError(storage.AddDeviceToMulticastGroup(storage.DB(), d.DevEUI, mg.ID))
		}

		ts.T().Run("Create", func(t *testing.T) {
			assert := require.New(t)

			qi1 := ns.MulticastQueueItem{
				MulticastGroupId: mg.ID.Bytes(),
				FCnt:             10,
				FPort:            20,
				FrmPayload:       []byte{1, 2, 3, 4},
			}
			qi2 := ns.MulticastQueueItem{
				MulticastGroupId: mg.ID.Bytes(),
				FCnt:             11,
				FPort:            20,
				FrmPayload:       []byte{1, 2, 3, 4},
			}

			_, err := ts.api.EnqueueMulticastQueueItem(context.Background(), &ns.EnqueueMulticastQueueItemRequest{
				MulticastQueueItem: &qi1,
			})
			assert.NoError(err)
			_, err = ts.api.EnqueueMulticastQueueItem(context.Background(), &ns.EnqueueMulticastQueueItemRequest{
				MulticastQueueItem: &qi2,
			})
			assert.NoError(err)

			t.Run("List", func(t *testing.T) {
				assert := require.New(t)

				listResp, err := ts.api.GetMulticastQueueItemsForMulticastGroup(context.Background(), &ns.GetMulticastQueueItemsForMulticastGroupRequest{
					MulticastGroupId: mg.ID.Bytes(),
				})
				assert.NoError(err)
				assert.Len(listResp.MulticastQueueItems, 2)

				for i, exp := range []struct {
					FCnt uint32
				}{
					{10}, {11},
				} {
					assert.Equal(exp.FCnt, listResp.MulticastQueueItems[i].FCnt)
				}
			})

			t.Run("Test emit and schedule at", func(t *testing.T) {
				assert := require.New(t)

				items, err := storage.GetMulticastQueueItemsForMulticastGroup(storage.DB(), mg.ID)
				assert.NoError(err)
				assert.Len(items, 4)

				for _, item := range items {
					assert.NotNil(item.EmitAtTimeSinceGPSEpoch)
				}

				emitAt := *items[0].EmitAtTimeSinceGPSEpoch

				// iterate over the enqueued items and based on the first item
				// calculate the next ping-slot and validate if this is used
				// for the next queue-item.
				for i := range items {
					if i == 0 {
						continue
					}
					var err error
					emitAt, err = classb.GetNextPingSlotAfter(emitAt, mg.MCAddr, (1<<12)/mg.PingSlotPeriod)
					assert.NoError(err)
					assert.Equal(emitAt, *items[i].EmitAtTimeSinceGPSEpoch, "queue item %d", i)

					scheduleAt := time.Time(gps.NewFromTimeSinceGPSEpoch(emitAt)).Add(-2 * config.C.NetworkServer.Scheduler.SchedulerInterval)
					assert.EqualValues(scheduleAt.UTC(), items[i].ScheduleAt.UTC())
				}
			})
		})
	})

	ts.T().Run("Class-C", func(t *testing.T) {
		assert := require.New(t)

		mg := storage.MulticastGroup{
			GroupType:        storage.MulticastGroupC,
			ServiceProfileID: sp.ID,
			RoutingProfileID: rp.ID,
		}
		assert.NoError(storage.CreateMulticastGroup(storage.DB(), &mg))

		for _, d := range devices {
			assert.NoError(storage.AddDeviceToMulticastGroup(storage.DB(), d.DevEUI, mg.ID))
		}

		ts.T().Run("Create", func(t *testing.T) {
			assert := require.New(t)

			qi1 := ns.MulticastQueueItem{
				MulticastGroupId: mg.ID.Bytes(),
				FCnt:             10,
				FPort:            20,
				FrmPayload:       []byte{1, 2, 3, 4},
			}
			qi2 := ns.MulticastQueueItem{
				MulticastGroupId: mg.ID.Bytes(),
				FCnt:             11,
				FPort:            20,
				FrmPayload:       []byte{1, 2, 3, 4},
			}

			_, err := ts.api.EnqueueMulticastQueueItem(context.Background(), &ns.EnqueueMulticastQueueItemRequest{
				MulticastQueueItem: &qi1,
			})
			assert.NoError(err)
			_, err = ts.api.EnqueueMulticastQueueItem(context.Background(), &ns.EnqueueMulticastQueueItemRequest{
				MulticastQueueItem: &qi2,
			})
			assert.NoError(err)

			t.Run("List", func(t *testing.T) {
				assert := require.New(t)

				listResp, err := ts.api.GetMulticastQueueItemsForMulticastGroup(context.Background(), &ns.GetMulticastQueueItemsForMulticastGroupRequest{
					MulticastGroupId: mg.ID.Bytes(),
				})
				assert.NoError(err)
				assert.Len(listResp.MulticastQueueItems, 2)

				for i, exp := range []struct {
					FCnt uint32
				}{
					{10}, {11},
				} {
					assert.Equal(exp.FCnt, listResp.MulticastQueueItems[i].FCnt)
				}
			})

			t.Run("Test emit and schedule at", func(t *testing.T) {
				assert := require.New(t)

				items, err := storage.GetMulticastQueueItemsForMulticastGroup(storage.DB(), mg.ID)
				assert.NoError(err)
				assert.Len(items, 4)

				for _, item := range items {
					assert.Nil(item.EmitAtTimeSinceGPSEpoch)
				}

				scheduleAt := items[0].ScheduleAt

				for i := range items {
					if i == 0 {
						continue
					}
					lockDuration := config.C.NetworkServer.Scheduler.ClassC.DownlinkLockDuration
					assert.Equal(scheduleAt, items[i].ScheduleAt.Add(-lockDuration))
					scheduleAt = items[i].ScheduleAt
				}
			})
		})
	})

}

func (ts *NetworkServerAPITestSuite) TestDevice() {
	assert := require.New(ts.T())

	rp := storage.RoutingProfile{}
	assert.NoError(storage.CreateRoutingProfile(storage.DB(), &rp))

	sp := storage.ServiceProfile{
		DRMin: 3,
		DRMax: 6,
	}
	assert.NoError(storage.CreateServiceProfile(storage.DB(), &sp))

	dp := storage.DeviceProfile{
		FactoryPresetFreqs: []int{
			868100000,
			868300000,
			868500000,
		},
		RXDelay1:       3,
		RXDROffset1:    2,
		RXDataRate2:    5,
		RXFreq2:        868900000,
		PingSlotPeriod: 32,
		PingSlotFreq:   868100000,
		PingSlotDR:     5,
		MACVersion:     "1.0.2",
	}
	assert.NoError(storage.CreateDeviceProfile(storage.DB(), &dp))

	devEUI := lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}

	ts.T().Run("Create", func(t *testing.T) {
		assert := require.New(t)

		d := &ns.Device{
			DevEui:            devEUI[:],
			DeviceProfileId:   dp.ID.Bytes(),
			ServiceProfileId:  sp.ID.Bytes(),
			RoutingProfileId:  rp.ID.Bytes(),
			SkipFCntCheck:     true,
			ReferenceAltitude: 5.6,
		}

		_, err := ts.api.CreateDevice(context.Background(), &ns.CreateDeviceRequest{
			Device: d,
		})
		assert.NoError(err)

		t.Run("Get", func(t *testing.T) {
			assert := require.New(t)

			getResp, err := ts.api.GetDevice(context.Background(), &ns.GetDeviceRequest{
				DevEui: devEUI[:],
			})
			assert.NoError(err)
			assert.Equal(d, getResp.Device)
		})

		t.Run("Multicast-groups", func(t *testing.T) {
			assert := require.New(t)

			mg1 := storage.MulticastGroup{
				RoutingProfileID: rp.ID,
				ServiceProfileID: sp.ID,
			}
			assert.NoError(storage.CreateMulticastGroup(storage.DB(), &mg1))

			t.Run("Add", func(t *testing.T) {
				_, err := ts.api.AddDeviceToMulticastGroup(context.Background(), &ns.AddDeviceToMulticastGroupRequest{
					DevEui:           devEUI[:],
					MulticastGroupId: mg1.ID.Bytes(),
				})
				assert.NoError(err)

				t.Run("Remove", func(t *testing.T) {
					assert := require.New(t)

					_, err := ts.api.RemoveDeviceFromMulticastGroup(context.Background(), &ns.RemoveDeviceFromMulticastGroupRequest{
						DevEui:           devEUI[:],
						MulticastGroupId: mg1.ID.Bytes(),
					})
					assert.NoError(err)

					_, err = ts.api.RemoveDeviceFromMulticastGroup(context.Background(), &ns.RemoveDeviceFromMulticastGroupRequest{
						DevEui:           devEUI[:],
						MulticastGroupId: mg1.ID.Bytes(),
					})
					assert.Error(err)
					assert.Equal(codes.NotFound, grpc.Code(err))
				})
			})
		})

		t.Run("Activate", func(t *testing.T) {
			assert := require.New(t)

			devEUI := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
			devAddr := [4]byte{6, 2, 3, 4}
			sNwkSIntKey := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
			fNwkSIntKey := [16]byte{2, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
			nwkSEncKey := [16]byte{3, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

			_, err = ts.api.ActivateDevice(context.Background(), &ns.ActivateDeviceRequest{
				DeviceActivation: &ns.DeviceActivation{
					DevEui:        devEUI[:],
					DevAddr:       devAddr[:],
					SNwkSIntKey:   sNwkSIntKey[:],
					FNwkSIntKey:   fNwkSIntKey[:],
					NwkSEncKey:    nwkSEncKey[:],
					FCntUp:        10,
					NFCntDown:     11,
					AFCntDown:     12,
					SkipFCntCheck: false,
				},
			})
			assert.NoError(err)

			t.Run("Enqueue with different security-context", func(t *testing.T) {
				assert := require.New(t)

				// create item in the queue (device is not activated yet)
				_, err := ts.api.CreateDeviceQueueItem(context.Background(), &ns.CreateDeviceQueueItemRequest{
					Item: &ns.DeviceQueueItem{
						DevAddr:    []byte{1, 2, 3, 4},
						DevEui:     devEUI[:],
						FrmPayload: []byte{1, 2, 3, 4},
						FCnt:       10,
						FPort:      20,
					},
				})
				assert.NotNil(err)
				assert.Equal(codes.InvalidArgument, grpc.Code(err))
			})

			t.Run("Enqueue with valid security-context", func(t *testing.T) {
				assert := require.New(t)

				// create item in the queue (device is not activated yet)
				_, err := ts.api.CreateDeviceQueueItem(context.Background(), &ns.CreateDeviceQueueItemRequest{
					Item: &ns.DeviceQueueItem{
						DevAddr:    []byte{6, 2, 3, 4},
						DevEui:     devEUI[:],
						FrmPayload: []byte{1, 2, 3, 4},
						FCnt:       10,
						FPort:      20,
					},
				})
				assert.Nil(err)
			})

			_, err = ts.api.ActivateDevice(context.Background(), &ns.ActivateDeviceRequest{
				DeviceActivation: &ns.DeviceActivation{
					DevEui:        devEUI[:],
					DevAddr:       devAddr[:],
					SNwkSIntKey:   sNwkSIntKey[:],
					FNwkSIntKey:   fNwkSIntKey[:],
					NwkSEncKey:    nwkSEncKey[:],
					FCntUp:        10,
					NFCntDown:     11,
					AFCntDown:     12,
					SkipFCntCheck: false,
				},
			})
			assert.NoError(err)

			t.Run("Device-queue is flushed", func(t *testing.T) {
				assert := require.New(t)
				items, err := storage.GetDeviceQueueItemsForDevEUI(storage.DB(), devEUI)
				assert.NoError(err)
				assert.Len(items, 0)
			})

			t.Run("Device-session is created", func(t *testing.T) {
				ds, err := storage.GetDeviceSession(storage.RedisPool(), devEUI)
				assert.NoError(err)
				assert.Equal(storage.DeviceSession{
					DeviceProfileID:  dp.ID,
					ServiceProfileID: sp.ID,
					RoutingProfileID: rp.ID,

					DevAddr:               devAddr,
					DevEUI:                devEUI,
					SNwkSIntKey:           sNwkSIntKey,
					FNwkSIntKey:           fNwkSIntKey,
					NwkSEncKey:            nwkSEncKey,
					FCntUp:                10,
					NFCntDown:             11,
					AFCntDown:             12,
					SkipFCntValidation:    true,
					EnabledUplinkChannels: band.Band().GetEnabledUplinkChannelIndices(),
					ChannelFrequencies:    []int{868100000, 868300000, 868500000},
					ExtraUplinkChannels:   map[int]loraband.Channel{},
					RXDelay:               3,
					RX1DROffset:           2,
					RX2DR:                 5,
					RX2Frequency:          868900000,
					UplinkGatewayHistory:  make(map[lorawan.EUI64]storage.UplinkGatewayHistory),
					PingSlotNb:            128,
					PingSlotDR:            5,
					PingSlotFrequency:     868100000,
					NbTrans:               1,
					MACVersion:            "1.0.2",
				}, ds)
			})

			t.Run("GetDeviceActivation", func(t *testing.T) {
				assert := require.New(t)

				resp, err := ts.api.GetDeviceActivation(context.Background(), &ns.GetDeviceActivationRequest{DevEui: devEUI[:]})
				assert.NoError(err)
				assert.Equal(&ns.DeviceActivation{
					DevEui:        devEUI[:],
					DevAddr:       devAddr[:],
					SNwkSIntKey:   sNwkSIntKey[:],
					FNwkSIntKey:   fNwkSIntKey[:],
					NwkSEncKey:    nwkSEncKey[:],
					FCntUp:        10,
					NFCntDown:     11,
					AFCntDown:     12,
					SkipFCntCheck: true,
				}, resp.DeviceActivation)
			})

			t.Run("GetNextDownlinkFCntForDevEUI", func(t *testing.T) {
				t.Run("LoRaWAN 1.0", func(t *testing.T) {
					assert := require.New(t)
					resp, err := ts.api.GetNextDownlinkFCntForDevEUI(context.Background(), &ns.GetNextDownlinkFCntForDevEUIRequest{DevEui: devEUI[:]})
					assert.NoError(err)
					assert.EqualValues(11, resp.FCnt)
				})

				t.Run("LoRaWAN 1.1", func(t *testing.T) {
					assert := require.New(t)
					ds, err := storage.GetDeviceSession(storage.RedisPool(), devEUI)
					assert.NoError(err)
					ds.MACVersion = "1.1.0"
					assert.NoError(storage.SaveDeviceSession(storage.RedisPool(), ds))

					resp, err := ts.api.GetNextDownlinkFCntForDevEUI(context.Background(), &ns.GetNextDownlinkFCntForDevEUIRequest{DevEui: devEUI[:]})
					assert.NoError(err)
					assert.EqualValues(12, resp.FCnt)
				})

				t.Run("With item in device-queue", func(t *testing.T) {
					assert := require.New(t)
					_, err := ts.api.CreateDeviceQueueItem(context.Background(), &ns.CreateDeviceQueueItemRequest{
						Item: &ns.DeviceQueueItem{
							DevEui:     devEUI[:],
							FrmPayload: []byte{1, 2, 3, 4},
							FCnt:       13,
							FPort:      20,
						},
					})
					assert.NoError(err)

					resp, err := ts.api.GetNextDownlinkFCntForDevEUI(context.Background(), &ns.GetNextDownlinkFCntForDevEUIRequest{DevEui: devEUI[:]})
					assert.NoError(err)
					assert.EqualValues(14, resp.FCnt)
				})
			})

			t.Run("DeactivateDevice", func(t *testing.T) {
				assert := require.New(t)

				items, err := storage.GetDeviceQueueItemsForDevEUI(storage.DB(), devEUI)
				assert.NoError(err)
				assert.Len(items, 1)

				_, err = ts.api.DeactivateDevice(context.Background(), &ns.DeactivateDeviceRequest{
					DevEui: devEUI[:],
				})
				assert.NoError(err)

				_, err = ts.api.GetDeviceActivation(context.Background(), &ns.GetDeviceActivationRequest{DevEui: devEUI[:]})
				assert.Equal(codes.NotFound, grpc.Code(err))

				items, err = storage.GetDeviceQueueItemsForDevEUI(storage.DB(), devEUI)
				assert.NoError(err)
				assert.Len(items, 0)
			})

			t.Run("Activate with Device.SkipFCntCheck set to true", func(t *testing.T) {
				d.SkipFCntCheck = true
				_, err := ts.api.UpdateDevice(context.Background(), &ns.UpdateDeviceRequest{
					Device: d,
				})
				assert.NoError(err)

				_, err = ts.api.ActivateDevice(context.Background(), &ns.ActivateDeviceRequest{
					DeviceActivation: &ns.DeviceActivation{
						DevEui:        devEUI[:],
						DevAddr:       devAddr[:],
						SNwkSIntKey:   sNwkSIntKey[:],
						FNwkSIntKey:   fNwkSIntKey[:],
						NwkSEncKey:    nwkSEncKey[:],
						FCntUp:        10,
						NFCntDown:     11,
						AFCntDown:     12,
						SkipFCntCheck: false,
					},
				})
				assert.NoError(err)

				ds, err := storage.GetDeviceSession(storage.RedisPool(), devEUI)
				assert.NoError(err)
				assert.True(ds.SkipFCntValidation)
			})

			t.Run("Device mode", func(t *testing.T) {
				tests := []struct {
					Name            string
					ClassBSupported bool
					ClassCSupported bool
					MACVersion      string
					ExpectedMode    storage.DeviceMode
				}{
					{
						Name:         "LoRaWAN 1.0 Class A supported",
						MACVersion:   "1.0.3",
						ExpectedMode: storage.DeviceModeA,
					},
					{
						Name:            "LoRaWAN 1.0 Class B supported",
						MACVersion:      "1.0.3",
						ClassBSupported: true,
						ExpectedMode:    storage.DeviceModeA,
					},
					{
						Name:            "LoRaWAN 1.0 Class C supported",
						MACVersion:      "1.0.3",
						ClassCSupported: true,
						ExpectedMode:    storage.DeviceModeC,
					},
					{
						Name:         "LoRaWAN 1.1 Class A supported",
						MACVersion:   "1.1.0",
						ExpectedMode: storage.DeviceModeA,
					},
					{
						Name:            "LoRaWAN 1.1 Class B supported",
						MACVersion:      "1.1.0",
						ClassBSupported: true,
						ExpectedMode:    storage.DeviceModeA,
					},
					{
						Name:            "LoRaWAN 1.1 Class C supported",
						MACVersion:      "1.1.0",
						ClassCSupported: true,
						ExpectedMode:    storage.DeviceModeA,
					},
				}

				for _, tst := range tests {
					t.Run(tst.Name, func(t *testing.T) {
						assert := require.New(t)
						dp.SupportsClassB = tst.ClassBSupported
						dp.SupportsClassC = tst.ClassCSupported
						dp.MACVersion = tst.MACVersion

						assert.NoError(storage.UpdateDeviceProfile(storage.DB(), &dp))

						_, err = ts.api.ActivateDevice(context.Background(), &ns.ActivateDeviceRequest{
							DeviceActivation: &ns.DeviceActivation{
								DevEui:      devEUI[:],
								DevAddr:     devAddr[:],
								SNwkSIntKey: sNwkSIntKey[:],
								FNwkSIntKey: fNwkSIntKey[:],
								NwkSEncKey:  nwkSEncKey[:],
							},
						})
						assert.NoError(err)

						d, err := storage.GetDevice(storage.DB(), devEUI)
						assert.NoError(err)
						assert.Equal(tst.ExpectedMode, d.Mode)
					})
				}
			})
		})

		t.Run("Update", func(t *testing.T) {
			assert := require.New(t)

			rp2 := storage.RoutingProfile{}
			assert.NoError(storage.CreateRoutingProfile(storage.DB(), &rp2))

			d.RoutingProfileId = rp2.ID.Bytes()
			_, err := ts.api.UpdateDevice(context.Background(), &ns.UpdateDeviceRequest{
				Device: d,
			})
			assert.NoError(err)

			getResp, err := ts.api.GetDevice(context.Background(), &ns.GetDeviceRequest{
				DevEui: devEUI[:],
			})
			assert.NoError(err)
			assert.Equal(d, getResp.Device)
		})

		t.Run("Delete", func(t *testing.T) {
			assert := require.New(t)

			_, err := ts.api.DeleteDevice(context.Background(), &ns.DeleteDeviceRequest{
				DevEui: devEUI[:],
			})
			assert.NoError(err)

			_, err = ts.api.DeleteDevice(context.Background(), &ns.DeleteDeviceRequest{
				DevEui: devEUI[:],
			})
			assert.Error(err)
			assert.Equal(codes.NotFound, grpc.Code(err))
		})
	})
}

func TestNetworkServerAPINew(t *testing.T) {
	suite.Run(t, new(NetworkServerAPITestSuite))
}
