/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mirbft

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	pb "github.com/IBM/mirbft/mirbftpb"

	"go.uber.org/zap"
)

var _ = Describe("Integration", func() {
	var (
		serializer      *serializer
		stateMachineVal *stateMachine
		epochConfig     *pb.EpochConfig
		networkConfig   *pb.NetworkConfig
		consumerConfig  *Config
		logger          *zap.Logger

		doneC chan struct{}
	)

	BeforeEach(func() {
		var err error
		logger, err = zap.NewDevelopment()
		Expect(err).NotTo(HaveOccurred())

		consumerConfig = &Config{
			ID:     0,
			Logger: logger,
			BatchParameters: BatchParameters{
				CutSizeBytes: 1,
			},
		}

		doneC = make(chan struct{})
	})

	AfterEach(func() {
		logger.Sync()
		close(doneC)
	})

	Describe("F=0,N=1", func() {
		BeforeEach(func() {
			epochConfig = &pb.EpochConfig{
				Number:             3,
				Leaders:            []uint64{0},
				StartingCheckpoint: &pb.Checkpoint{},
			}

			networkConfig = &pb.NetworkConfig{
				CheckpointInterval: 2,
				F:                  0,
				Nodes:              []uint64{0},
			}

			stateMachineVal = newStateMachine(networkConfig, consumerConfig)
			stateMachineVal.activeEpoch = newEpoch(epochConfig, stateMachineVal.checkpointTracker, nil, networkConfig, consumerConfig)
			stateMachineVal.nodeMsgs[0].setActiveEpoch(stateMachineVal.activeEpoch)

			serializer = newSerializer(stateMachineVal, doneC)
		})

		It("works from proposal through commit", func() {
			By("proposing a message")
			serializer.propC <- []byte("data")
			actions := &Actions{}
			Eventually(serializer.actionsC).Should(Receive(actions))
			Expect(actions).To(Equal(&Actions{
				Preprocess: []Proposal{
					{
						Source: 0,
						Data:   []byte("data"),
					},
				},
			}))

			By("returning a processed version of the proposal")
			serializer.resultsC <- ActionResults{
				Preprocesses: []PreprocessResult{
					{
						Cup: 7,
						Proposal: Proposal{
							Source: 0,
							Data:   []byte("data"),
						},
					},
				},
			}
			Eventually(serializer.actionsC).Should(Receive(actions))
			Expect(actions).To(Equal(&Actions{
				Broadcast: []*pb.Msg{
					{
						Type: &pb.Msg_Preprepare{
							Preprepare: &pb.Preprepare{
								Epoch: 3,
								SeqNo: 1,
								Batch: [][]byte{[]byte("data")},
							},
						},
					},
				},
			}))

			By("broadcasting the preprepare to myself")
			serializer.stepC <- step{
				Source: 0,
				Msg:    actions.Broadcast[0],
			}
			Eventually(serializer.actionsC).Should(Receive(actions))
			Expect(actions).To(Equal(&Actions{
				Digest: []*Entry{
					{
						Epoch: 3,
						SeqNo: 1,
						Batch: [][]byte{[]byte("data")},
					},
				},
			}))

			By("returning a digest for the batch")
			serializer.resultsC <- ActionResults{
				Digests: []DigestResult{
					{
						Entry: &Entry{
							Epoch: 3,
							SeqNo: 1,
							Batch: [][]byte{[]byte("data")},
						},
						Digest: []byte("fake-digest"),
					},
				},
			}
			Eventually(serializer.actionsC).Should(Receive(actions))
			Expect(actions).To(Equal(&Actions{
				Broadcast: []*pb.Msg{
					{
						Type: &pb.Msg_Commit{
							Commit: &pb.Commit{
								Epoch:  3,
								SeqNo:  1,
								Digest: []byte(("fake-digest")),
							},
						},
					},
				},
			}))

			By("broadcasting the commit to myself")
			serializer.stepC <- step{
				Source: 0,
				Msg:    actions.Broadcast[0],
			}
			Eventually(serializer.actionsC).Should(Receive(actions))
			Expect(actions).To(Equal(&Actions{
				Commit: []*Entry{
					{
						Epoch: 3,
						SeqNo: 1,
						Batch: [][]byte{[]byte("data")},
					},
				},
			}))
		})
	})

	Describe("F=1,N=4", func() {
		BeforeEach(func() {
			epochConfig = &pb.EpochConfig{
				Number:             3,
				Leaders:            []uint64{0, 1, 2, 3},
				StartingCheckpoint: &pb.Checkpoint{},
			}

			networkConfig = &pb.NetworkConfig{
				CheckpointInterval: 5,
				F:                  1,
				Nodes:              []uint64{0, 1, 2, 3},
			}

			stateMachineVal = newStateMachine(networkConfig, consumerConfig)
			stateMachineVal.activeEpoch = newEpoch(epochConfig, stateMachineVal.checkpointTracker, nil, networkConfig, consumerConfig)
			stateMachineVal.nodeMsgs[0].setActiveEpoch(stateMachineVal.activeEpoch)
			stateMachineVal.nodeMsgs[1].setActiveEpoch(stateMachineVal.activeEpoch)
			stateMachineVal.nodeMsgs[2].setActiveEpoch(stateMachineVal.activeEpoch)
			stateMachineVal.nodeMsgs[3].setActiveEpoch(stateMachineVal.activeEpoch)

			serializer = newSerializer(stateMachineVal, doneC)

		})

		It("works from proposal through commit", func() {
			By("proposing a message")
			serializer.propC <- []byte("data")
			actions := &Actions{}
			Eventually(serializer.actionsC).Should(Receive(actions))
			Expect(actions).To(Equal(&Actions{
				Preprocess: []Proposal{
					{
						Source: 0,
						Data:   []byte("data"),
					},
				},
			}))

			By("returning a processed version of the proposal")
			serializer.resultsC <- ActionResults{
				Preprocesses: []PreprocessResult{
					{
						Cup: 7,
						Proposal: Proposal{
							Source: 0,
							Data:   []byte("data"),
						},
					},
				},
			}
			Eventually(serializer.actionsC).Should(Receive(actions))
			Expect(actions).To(Equal(&Actions{
				Unicast: []Unicast{
					{
						Target: 3,
						Msg: &pb.Msg{
							Type: &pb.Msg_Forward{
								Forward: &pb.Forward{
									Epoch:  3,
									Bucket: 3,
									Data:   []byte("data"),
								},
							},
						},
					},
				},
			}))

			By("faking a preprepare from the leader")
			serializer.stepC <- step{
				Source: 3,
				Msg: &pb.Msg{
					Type: &pb.Msg_Preprepare{
						Preprepare: &pb.Preprepare{
							Epoch: 3,
							SeqNo: 4,
							Batch: [][]byte{[]byte("data")},
						},
					},
				},
			}
			Eventually(serializer.actionsC).Should(Receive(actions))
			Expect(actions).To(Equal(&Actions{
				Digest: []*Entry{
					{
						Epoch: 3,
						SeqNo: 4,
						Batch: [][]byte{[]byte("data")},
					},
				},
			}))

			By("returning a digest for the batch")
			serializer.resultsC <- ActionResults{
				Digests: []DigestResult{
					{
						Entry: &Entry{
							Epoch: 3,
							SeqNo: 4,
							Batch: [][]byte{[]byte("data")},
						},
						Digest: []byte("fake-digest"),
					},
				},
			}
			Eventually(serializer.actionsC).Should(Receive(actions))
			Expect(actions).To(Equal(&Actions{
				Validate: []*Entry{
					{
						Epoch: 3,
						SeqNo: 4,
						Batch: [][]byte{[]byte("data")},
					},
				},
			}))

			By("returning a successful validatation for the batch")
			serializer.resultsC <- ActionResults{
				Validations: []ValidateResult{
					{
						Entry: &Entry{
							Epoch: 3,
							SeqNo: 4,
							Batch: [][]byte{[]byte("data")},
						},
						Valid: true,
					},
				},
			}
			Eventually(serializer.actionsC).Should(Receive(actions))
			Expect(actions).To(Equal(&Actions{
				Broadcast: []*pb.Msg{
					{
						Type: &pb.Msg_Prepare{
							Prepare: &pb.Prepare{
								Epoch:  3,
								SeqNo:  4,
								Digest: []byte(("fake-digest")),
							},
						},
					},
				},
			}))

			By("broadcasting the prepare to myself, and from one other node")
			serializer.stepC <- step{
				Source: 0,
				Msg:    actions.Broadcast[0],
			}

			serializer.stepC <- step{
				Source: 1,
				Msg:    actions.Broadcast[0],
			}

			Eventually(serializer.actionsC).Should(Receive(actions))
			Expect(actions).To(Equal(&Actions{
				Broadcast: []*pb.Msg{
					{
						Type: &pb.Msg_Commit{
							Commit: &pb.Commit{
								Epoch:  3,
								SeqNo:  4,
								Digest: []byte(("fake-digest")),
							},
						},
					},
				},
			}))

			By("broadcasting the commit to myself, and from two other nodes")
			serializer.stepC <- step{
				Source: 0,
				Msg:    actions.Broadcast[0],
			}

			serializer.stepC <- step{
				Source: 1,
				Msg:    actions.Broadcast[0],
			}

			serializer.stepC <- step{
				Source: 3,
				Msg:    actions.Broadcast[0],
			}

			Eventually(serializer.actionsC).Should(Receive(actions))
			Expect(actions).To(Equal(&Actions{
				Commit: []*Entry{
					{
						Epoch: 3,
						SeqNo: 4,
						Batch: [][]byte{[]byte("data")},
					},
				},
			}))
		})
	})
})
