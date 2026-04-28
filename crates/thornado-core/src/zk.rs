use crate::{Error, Result, WithdrawalPublicInputs};

#[derive(Debug, Default, Clone)]
pub struct ZkProofVerifier;

impl crate::ProofVerifier for ZkProofVerifier {
    fn verify_withdrawal(
        &self,
        proof: &crate::WithdrawalProof,
        public: &WithdrawalPublicInputs,
    ) -> Result<()> {
        #[cfg(feature = "orchard-zcash")]
        {
            let orchard = proof.orchard.as_ref().ok_or(Error::InvalidProof)?;
            if orchard.anchor_hex != public.merkle_root {
                return Err(Error::InvalidProof);
            }
            let matching_nullifiers = orchard
                .actions
                .iter()
                .filter(|action| action.nullifier_hex == public.nullifier_hash)
                .count();
            if matching_nullifiers != 1 {
                return Err(Error::InvalidProof);
            }
            if orchard.value_balance.unsigned_abs() != public.denomination_sats {
                return Err(Error::InvalidProof);
            }
            let public_context = crate::orchard_public_context(public);
            crate::orchard::verify_orchard_withdrawal(orchard, &public_context)
        }
        #[cfg(not(feature = "orchard-zcash"))]
        {
            let _ = (proof, public);
            Err(Error::InvalidProof)
        }
    }

    fn reveals_commitment(&self) -> bool {
        false
    }
}
